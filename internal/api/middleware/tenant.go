package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
)

const (
	apiKeyHeader = "X-API-Key"
	cacheTTL     = 5 * time.Minute
)

// tenantCacheEntry stores a cached tenant lookup result.
type tenantCacheEntry struct {
	tenantID string
	isActive bool
	cachedAt time.Time
}

// TenantMiddleware resolves API keys to tenant IDs with in-memory caching.
type TenantMiddleware struct {
	pool  *pgxpool.Pool
	mu    sync.RWMutex
	cache map[string]*tenantCacheEntry
}

// NewTenantMiddleware creates a new TenantMiddleware.
func NewTenantMiddleware(pool *pgxpool.Pool) *TenantMiddleware {
	return &TenantMiddleware{
		pool:  pool,
		cache: make(map[string]*tenantCacheEntry),
	}
}

// Handler returns the HTTP middleware function that resolves API keys.
func (tm *TenantMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawKey := r.Header.Get(apiKeyHeader)
		if rawKey == "" {
			handlers.RespondError(w, http.StatusUnauthorized,
				"MISSING_API_KEY", "X-API-Key header is required")
			return
		}

		hashedKey := hashKey(rawKey)

		// Check cache first
		if entry, ok := tm.getFromCache(hashedKey); ok {
			if !entry.isActive {
				handlers.RespondError(w, http.StatusForbidden,
					"TENANT_INACTIVE", "Tenant is inactive")
				return
			}
			ctx := SetTenantID(r.Context(), entry.tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// DB lookup
		tenantID, isActive, err := tm.lookupTenant(r.Context(), hashedKey)
		if err != nil {
			handlers.RespondError(w, http.StatusUnauthorized,
				"INVALID_API_KEY", "Invalid API key")
			return
		}

		// Cache the result
		tm.setCache(hashedKey, tenantID, isActive)

		if !isActive {
			handlers.RespondError(w, http.StatusForbidden,
				"TENANT_INACTIVE", "Tenant is inactive")
			return
		}

		ctx := SetTenantID(r.Context(), tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// lookupTenant queries the database for a tenant by hashed API key.
func (tm *TenantMiddleware) lookupTenant(
	ctx context.Context, hashedKey string,
) (string, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var tenantID string
	var isActive bool
	err := tm.pool.QueryRow(ctx, `
		SELECT id, is_active FROM tenants WHERE api_key = $1
	`, hashedKey).Scan(&tenantID, &isActive)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", false, err
		}
		return "", false, err
	}
	return tenantID, isActive, nil
}

// getFromCache returns a cached entry if it exists and is not expired.
func (tm *TenantMiddleware) getFromCache(hashedKey string) (*tenantCacheEntry, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	entry, exists := tm.cache[hashedKey]
	if !exists {
		return nil, false
	}
	if time.Since(entry.cachedAt) > cacheTTL {
		return nil, false
	}
	return entry, true
}

// setCache stores a tenant lookup result in the cache.
func (tm *TenantMiddleware) setCache(hashedKey, tenantID string, isActive bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.cache[hashedKey] = &tenantCacheEntry{
		tenantID: tenantID,
		isActive: isActive,
		cachedAt: time.Now(),
	}
}

// hashKey hashes a raw API key with SHA-256.
func hashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

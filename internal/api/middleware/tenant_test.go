package middleware_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/middleware"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// testPoolDSN returns the pgx-compatible PostgreSQL connection string.
func testPoolDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fcl:localdev@localhost:5441/compliance_ledger?sslmode=disable"
	}
	return dsn
}

// newTestPool creates a pgxpool connection pool for testing.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testPoolDSN()

	err := store.RunMigrations(dsn, "file://../../../migrations")
	require.NoError(t, err, "failed to run migrations")

	pool, err := store.NewPostgresPool(dsn)
	require.NoError(t, err, "failed to create test pool")

	t.Cleanup(func() { pool.Close() })
	return pool
}

// hashAPIKey hashes a raw API key with SHA-256.
func hashAPIKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// seedTenantWithAPIKey inserts a tenant with a known hashed API key.
func seedTenantWithAPIKey(
	t *testing.T, pool *pgxpool.Pool,
	name, rawKey string, active bool,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	hashed := hashAPIKey(rawKey)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key, is_active)
		VALUES ($1, $2, $3, $4)
	`, id, name, hashed, active)
	require.NoError(t, err, "failed to seed tenant")
	return id
}

// cleanupTenant deletes a tenant by ID.
func cleanupTenant(t *testing.T, pool *pgxpool.Pool, ids ...uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, id := range ids {
		_, _ = pool.Exec(ctx, `DELETE FROM reports WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_events WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM escalation_rules WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM discrepancies WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	}
}

// ---------- Missing API key → 401 ----------

func TestTenantMiddleware_MissingAPIKey(t *testing.T) {
	pool := newTestPool(t)
	tm := middleware.NewTenantMiddleware(pool)

	handler := tm.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "MISSING_API_KEY")
}

// ---------- Invalid API key → 401 ----------

func TestTenantMiddleware_InvalidAPIKey(t *testing.T) {
	pool := newTestPool(t)
	tm := middleware.NewTenantMiddleware(pool)

	handler := tm.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-API-Key", "nonexistent-key-12345")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "INVALID_API_KEY")
}

// ---------- Inactive tenant → 403 ----------

func TestTenantMiddleware_InactiveTenant(t *testing.T) {
	pool := newTestPool(t)
	rawKey := fmt.Sprintf("inactive-key-%s", uuid.New().String()[:8])
	tenantID := seedTenantWithAPIKey(t, pool, "inactive-tenant", rawKey, false)
	t.Cleanup(func() { cleanupTenant(t, pool, tenantID) })

	tm := middleware.NewTenantMiddleware(pool)

	handler := tm.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-API-Key", rawKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "TENANT_INACTIVE")
}

// ---------- Valid API key → 200 + tenant_id in context ----------

func TestTenantMiddleware_ValidAPIKey(t *testing.T) {
	pool := newTestPool(t)
	rawKey := fmt.Sprintf("valid-key-%s", uuid.New().String()[:8])
	tenantID := seedTenantWithAPIKey(t, pool, "active-tenant", rawKey, true)
	t.Cleanup(func() { cleanupTenant(t, pool, tenantID) })

	tm := middleware.NewTenantMiddleware(pool)

	var capturedTenantID string
	handler := tm.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTenantID = middleware.GetTenantID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-API-Key", rawKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, tenantID.String(), capturedTenantID)
}

// ---------- Cache hit on second request ----------

func TestTenantMiddleware_CacheHit(t *testing.T) {
	pool := newTestPool(t)
	rawKey := fmt.Sprintf("cache-key-%s", uuid.New().String()[:8])
	tenantID := seedTenantWithAPIKey(t, pool, "cache-tenant", rawKey, true)
	t.Cleanup(func() { cleanupTenant(t, pool, tenantID) })

	tm := middleware.NewTenantMiddleware(pool)

	var capturedTenantID string
	handler := tm.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTenantID = middleware.GetTenantID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// First request — DB lookup
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-API-Key", rawKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, tenantID.String(), capturedTenantID)

	// Second request — should use cache (same result)
	capturedTenantID = ""
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-API-Key", rawKey)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, tenantID.String(), capturedTenantID)
}

// ---------- Empty API key treated as missing ----------

func TestTenantMiddleware_EmptyAPIKey(t *testing.T) {
	pool := newTestPool(t)
	tm := middleware.NewTenantMiddleware(pool)

	handler := tm.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-API-Key", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "MISSING_API_KEY")
}

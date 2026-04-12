package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
)

// TenantHandler provides HTTP handlers for tenant endpoints.
type TenantHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

// NewTenantHandler creates a new TenantHandler.
func NewTenantHandler(pool *pgxpool.Pool, cfg *config.Config) *TenantHandler {
	return &TenantHandler{
		pool: pool,
		cfg:  cfg,
	}
}

// tenantResponse is the JSON response for tenant info. It deliberately
// excludes the api_key field for security.
type tenantResponse struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	IsActive  bool                   `json:"is_active"`
	Settings  map[string]interface{} `json:"settings"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// registerRequest is the expected JSON body for tenant registration.
type registerRequest struct {
	Name string `json:"name"`
}

// registerResponse is the JSON response returned after tenant creation.
// The api_key is returned in plaintext ONLY at creation time.
type registerResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	APIKey string `json:"api_key"`
}

// GetMe returns the current tenant's info based on tenant_id in context.
func (h *TenantHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	tenantID := ctxutil.GetTenantID(r.Context())
	if tenantID == "" {
		RespondError(w, http.StatusUnauthorized,
			"MISSING_TENANT", "Tenant ID not found in context")
		return
	}

	tenant, err := h.queryTenant(r.Context(), tenantID)
	if err != nil {
		RespondError(w, http.StatusNotFound,
			"TENANT_NOT_FOUND", "Tenant not found")
		return
	}

	RespondJSON(w, http.StatusOK, tenant)
}

// queryTenant fetches a tenant from the database by ID, excluding api_key.
func (h *TenantHandler) queryTenant(
	ctx context.Context, tenantID string,
) (*tenantResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var t tenantResponse
	var settingsBytes []byte

	err := h.pool.QueryRow(ctx, `
		SELECT id, name, is_active, settings, created_at, updated_at
		FROM tenants WHERE id = $1
	`, tenantID).Scan(
		&t.ID, &t.Name, &t.IsActive, &settingsBytes,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("tenant not found: %s", tenantID)
		}
		return nil, fmt.Errorf("querying tenant: %w", err)
	}

	if settingsBytes != nil {
		_ = json.Unmarshal(settingsBytes, &t.Settings)
	}
	if t.Settings == nil {
		t.Settings = make(map[string]interface{})
	}

	return &t, nil
}

// Register handles self-registration of new tenants.
func (h *TenantHandler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.SelfRegistrationEnabled {
		RespondError(w, http.StatusForbidden,
			"REGISTRATION_DISABLED", "Tenant self-registration is disabled")
		return
	}

	var body registerRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_BODY", "Invalid JSON body")
		return
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		RespondError(w, http.StatusBadRequest,
			"MISSING_NAME", "Tenant name is required")
		return
	}
	if len(body.Name) > 255 {
		RespondError(w, http.StatusBadRequest,
			"NAME_TOO_LONG", "Tenant name must be 255 characters or fewer")
		return
	}

	plainKey, hashedKey, err := generateAPIKey()
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"KEY_GENERATION_FAILED", "Failed to generate API key")
		return
	}

	tenantID, err := h.insertTenant(r.Context(), body.Name, hashedKey)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"REGISTRATION_FAILED", "Failed to register tenant")
		return
	}

	RespondJSON(w, http.StatusCreated, registerResponse{
		ID:     tenantID,
		Name:   body.Name,
		APIKey: plainKey,
	})
}

// insertTenant creates a new tenant row and returns the generated UUID.
func (h *TenantHandler) insertTenant(
	ctx context.Context, name, hashedKey string,
) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var id string
	err := h.pool.QueryRow(ctx, `
		INSERT INTO tenants (name, api_key, is_active)
		VALUES ($1, $2, true)
		RETURNING id
	`, name, hashedKey).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting tenant: %w", err)
	}
	return id, nil
}

// generateAPIKey generates a new API key with the fcl_live_ prefix.
// Returns the plaintext key and SHA-256 hash for storage.
func generateAPIKey() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating random bytes: %w", err)
	}

	plainKey := "fcl_live_" + hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(plainKey))
	hashedKey := hex.EncodeToString(hash[:])

	return plainKey, hashedKey, nil
}

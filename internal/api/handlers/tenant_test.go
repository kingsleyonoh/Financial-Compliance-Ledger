package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
)

// ---------- GetMe Tests ----------

func TestTenantHandler_GetMe_ReturnsTenantInfo(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	// Seed a tenant directly
	tenantID := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key, is_active, settings)
		VALUES ($1, $2, $3, true, '{"theme":"dark"}')
	`, tenantID, "Test Corp", "hashed-key-getme")
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	// Set tenant_id in context (simulating middleware)
	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/me", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.GetMe(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err = json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, tenantID.String(), body["id"])
	assert.Equal(t, "Test Corp", body["name"])
	assert.Equal(t, true, body["is_active"])
	assert.NotNil(t, body["settings"])
	assert.NotNil(t, body["created_at"])
	assert.NotNil(t, body["updated_at"])
}

func TestTenantHandler_GetMe_ExcludesAPIKey(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	tenantID := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key, is_active)
		VALUES ($1, $2, $3, true)
	`, tenantID, "Key Excluded Corp", "hashed-key-excluded")
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/me", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.GetMe(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err = json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	// api_key must NEVER be in the response
	_, hasAPIKey := body["api_key"]
	assert.False(t, hasAPIKey, "api_key must not be present in response")
}

func TestTenantHandler_GetMe_NoTenantIDReturns401(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	// No tenant_id set in context
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/me", nil)
	rec := httptest.NewRecorder()

	h.GetMe(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestTenantHandler_GetMe_NonExistentTenantReturns404(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	reqCtx := ctxutil.SetTenantID(context.Background(), uuid.New().String())
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/me", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.GetMe(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ---------- Register Tests ----------

func TestTenantHandler_Register_Success(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	body := map[string]string{"name": "New Tenant Corp"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tenants/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	// Must contain id, name, api_key
	assert.NotEmpty(t, resp["id"], "response must contain id")
	assert.Equal(t, "New Tenant Corp", resp["name"])

	// API key must be returned in plaintext at creation
	apiKey, ok := resp["api_key"].(string)
	require.True(t, ok, "api_key must be a string")
	assert.True(t, len(apiKey) > 0, "api_key must not be empty")
	assert.Contains(t, apiKey, "fcl_live_", "api_key must start with fcl_live_")

	// Cleanup
	if id, ok := resp["id"].(string); ok {
		cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM tenants WHERE id = $1`, id)
	}
}

func TestTenantHandler_Register_DisabledReturns403(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: false}
	h := handlers.NewTenantHandler(pool, &cfg)

	body := map[string]string{"name": "Disabled Corp"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tenants/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestTenantHandler_Register_MissingNameReturns400(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	body := map[string]string{}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tenants/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTenantHandler_Register_EmptyNameReturns400(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	body := map[string]string{"name": ""}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tenants/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTenantHandler_Register_DuplicateNameAllowed(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	name := fmt.Sprintf("Duplicate Corp %d", time.Now().UnixNano())

	// Register first tenant
	body1, _ := json.Marshal(map[string]string{"name": name})
	req1 := httptest.NewRequest(http.MethodPost, "/api/tenants/register", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	h.Register(rec1, req1)
	require.Equal(t, http.StatusCreated, rec1.Code)

	// Register second tenant with same name
	body2, _ := json.Marshal(map[string]string{"name": name})
	req2 := httptest.NewRequest(http.MethodPost, "/api/tenants/register", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	h.Register(rec2, req2)
	assert.Equal(t, http.StatusCreated, rec2.Code)

	// Different API keys
	var resp1, resp2 map[string]interface{}
	_ = json.NewDecoder(rec1.Body).Decode(&resp1)
	_ = json.NewDecoder(rec2.Body).Decode(&resp2)
	assert.NotEqual(t, resp1["api_key"], resp2["api_key"])

	// Cleanup
	cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	if id, ok := resp1["id"].(string); ok {
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM tenants WHERE id = $1`, id)
	}
	if id, ok := resp2["id"].(string); ok {
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM tenants WHERE id = $1`, id)
	}
}

func TestTenantHandler_Register_APIKeyFormat(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	body, _ := json.Marshal(map[string]string{"name": "Format Check Corp"})
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	apiKey := resp["api_key"].(string)
	// Format: fcl_live_ + 32 hex chars = 9 + 32 = 41 total
	assert.Len(t, apiKey, 9+64, "api_key should be fcl_live_ prefix + 64 hex chars (32 random bytes)")
	assert.Equal(t, "fcl_live_", apiKey[:9])

	// Cleanup
	if id, ok := resp["id"].(string); ok {
		cleanupCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM tenants WHERE id = $1`, id)
	}
}

func TestTenantHandler_Register_InvalidJSONReturns400(t *testing.T) {
	pool := newTestPool(t)
	cfg := config.Config{SelfRegistrationEnabled: true}
	h := handlers.NewTenantHandler(pool, &cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/tenants/register",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

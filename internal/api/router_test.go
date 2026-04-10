package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api"
)

// ---------- Router creates without panic ----------

func TestNewRouter_CreatesWithoutPanic(t *testing.T) {
	deps := api.RouterDeps{
		Logger: zerolog.Nop(),
	}

	assert.NotPanics(t, func() {
		r := api.NewRouter(deps)
		assert.NotNil(t, r)
	})
}

// ---------- Health check returns 200 (no auth required) ----------

func TestRouter_HealthCheck(t *testing.T) {
	deps := api.RouterDeps{
		Logger: zerolog.Nop(),
	}
	r := api.NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body["status"])
}

// ---------- API routes without auth return 401 ----------

func TestRouter_APIRoutesRequireAuth(t *testing.T) {
	deps := api.RouterDeps{
		Logger: zerolog.Nop(),
	}
	r := api.NewRouter(deps)

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/discrepancies"},
		{http.MethodGet, "/api/rules"},
		{http.MethodGet, "/api/reports"},
		{http.MethodGet, "/api/stats"},
	}

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		// Without X-API-Key, should get 401
		assert.Equal(t, http.StatusUnauthorized, rec.Code,
			"%s %s should require auth", route.method, route.path)
	}
}

// ---------- Tenant registration endpoint exists (public) ----------

func TestRouter_TenantRegistrationRouteExists(t *testing.T) {
	deps := api.RouterDeps{
		Logger: zerolog.Nop(),
	}
	r := api.NewRouter(deps)

	req := httptest.NewRequest(http.MethodPost, "/api/tenants/register", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Should NOT be 404 (route exists) or 401 (public route)
	// Placeholder returns 501
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)
}

// ---------- 404 for unknown routes ----------

func TestRouter_UnknownRouteReturns404(t *testing.T) {
	deps := api.RouterDeps{
		Logger: zerolog.Nop(),
	}
	r := api.NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

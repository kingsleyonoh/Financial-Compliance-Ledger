package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// ---------- GetByID Tests ----------

func TestDiscrepancyHandler_GetByID_ReturnsDiscrepancy(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "getbyid-tenant")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-get-001", domain.SeverityHigh, domain.StatusOpen)

	// Also seed an event for the timeline
	seedHandlerEvent(t, pool, tenantID, discID, domain.EventReceived)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/discrepancies/%s", discID.String()), nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", discID.String())
	req = req.WithContext(context.WithValue(
		req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	disc, ok := body["discrepancy"].(map[string]interface{})
	require.True(t, ok, "response must contain discrepancy object")
	assert.Equal(t, discID.String(), disc["id"])

	events, ok := body["events"].([]interface{})
	require.True(t, ok, "response must contain events array")
	assert.Len(t, events, 1)
}

func TestDiscrepancyHandler_GetByID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "getbyid-notfound")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	fakeID := uuid.New()
	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/discrepancies/%s", fakeID.String()), nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fakeID.String())
	req = req.WithContext(context.WithValue(
		req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDiscrepancyHandler_GetByID_InvalidUUID(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "getbyid-baduuid")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())

	req := httptest.NewRequest(http.MethodGet,
		"/api/discrepancies/not-a-uuid", nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req = req.WithContext(context.WithValue(
		req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDiscrepancyHandler_GetByID_NoTenantReturns401(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	req := httptest.NewRequest(http.MethodGet,
		"/api/discrepancies/"+uuid.New().String(), nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(
		req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestDiscrepancyHandler_GetByID_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantA := seedHandlerTenant(t, pool, "getid-iso-a")
	tenantB := seedHandlerTenant(t, pool, "getid-iso-b")
	t.Cleanup(func() {
		cleanupHandlerTenantData(t, pool, tenantA)
		cleanupHandlerTenantData(t, pool, tenantB)
	})

	discID := seedHandlerDiscrepancy(t, pool, tenantB,
		"ext-iso-b2", domain.SeverityHigh, domain.StatusOpen)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantA.String())
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/discrepancies/%s", discID.String()), nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", discID.String())
	req = req.WithContext(context.WithValue(
		req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.GetByID(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"tenant A must not see tenant B's data")
}

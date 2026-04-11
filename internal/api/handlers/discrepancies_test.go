package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

func TestDiscrepancyHandler_List_ReturnsDiscrepancies(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "list-disc-tenant")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	seedHandlerDiscrepancy(t, pool, tenantID, "ext-list-001",
		domain.SeverityHigh, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantID, "ext-list-002",
		domain.SeverityLow, domain.StatusOpen)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/discrepancies", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	discs, ok := body["discrepancies"].([]interface{})
	require.True(t, ok, "discrepancies must be an array")
	assert.Len(t, discs, 2)

	total, ok := body["total"].(float64)
	require.True(t, ok, "total must be a number")
	assert.Equal(t, float64(2), total)
}

func TestDiscrepancyHandler_List_EmptyReturnsEmptyArray(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "list-empty-tenant")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/discrepancies", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	discs, ok := body["discrepancies"].([]interface{})
	require.True(t, ok, "discrepancies must be an array, not null")
	assert.Len(t, discs, 0)
	assert.Equal(t, float64(0), body["total"])
}

func TestDiscrepancyHandler_List_FilterByStatus(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "list-filter-status")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	seedHandlerDiscrepancy(t, pool, tenantID, "ext-fs-001",
		domain.SeverityHigh, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantID, "ext-fs-002",
		domain.SeverityLow, domain.StatusAcknowledged)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/discrepancies?status=open", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	discs := body["discrepancies"].([]interface{})
	assert.Len(t, discs, 1)
	assert.Equal(t, float64(1), body["total"])
}

func TestDiscrepancyHandler_List_FilterBySeverity(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "list-filter-sev")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	seedHandlerDiscrepancy(t, pool, tenantID, "ext-sev-001",
		domain.SeverityHigh, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantID, "ext-sev-002",
		domain.SeverityLow, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantID, "ext-sev-003",
		domain.SeverityHigh, domain.StatusOpen)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/discrepancies?severity=high", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	discs := body["discrepancies"].([]interface{})
	assert.Len(t, discs, 2)
	assert.Equal(t, float64(2), body["total"])
}

func TestDiscrepancyHandler_List_LimitAndCapping(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "list-limit")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	for i := 0; i < 5; i++ {
		seedHandlerDiscrepancy(t, pool, tenantID,
			fmt.Sprintf("ext-lim-%03d", i),
			domain.SeverityLow, domain.StatusOpen)
	}

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())

	// Test limit=2 returns only 2 items but total reflects all 5
	req := httptest.NewRequest(http.MethodGet,
		"/api/discrepancies?limit=2", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)
	discs := body["discrepancies"].([]interface{})
	assert.Len(t, discs, 2)
	assert.Equal(t, float64(5), body["total"])
	// Test limit=999 is capped (returns all 5, not an error)
	req2 := httptest.NewRequest(http.MethodGet,
		"/api/discrepancies?limit=999", nil)
	req2 = req2.WithContext(reqCtx)
	rec2 := httptest.NewRecorder()
	h.List(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var body2 map[string]interface{}
	err = json.NewDecoder(rec2.Body).Decode(&body2)
	require.NoError(t, err)
	discs2 := body2["discrepancies"].([]interface{})
	assert.Len(t, discs2, 5)
}
func TestDiscrepancyHandler_List_CursorPagination(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "list-cursor")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	for i := 0; i < 5; i++ {
		seedHandlerDiscrepancy(t, pool, tenantID,
			fmt.Sprintf("ext-cur-%03d", i),
			domain.SeverityLow, domain.StatusOpen)
	}

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/discrepancies?limit=2", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var page1 map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&page1)
	require.NoError(t, err)

	discs1 := page1["discrepancies"].([]interface{})
	assert.Len(t, discs1, 2)

	cursor, ok := page1["cursor"].(string)
	require.True(t, ok, "cursor must be present for pagination")
	assert.NotEmpty(t, cursor)

	req2 := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/discrepancies?limit=2&cursor=%s", cursor), nil)
	req2 = req2.WithContext(reqCtx)
	rec2 := httptest.NewRecorder()

	h.List(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var page2 map[string]interface{}
	err = json.NewDecoder(rec2.Body).Decode(&page2)
	require.NoError(t, err)

	discs2 := page2["discrepancies"].([]interface{})
	assert.Len(t, discs2, 2)

	id1 := discs1[0].(map[string]interface{})["id"]
	id2 := discs2[0].(map[string]interface{})["id"]
	assert.NotEqual(t, id1, id2, "pages must not overlap")
}

func TestDiscrepancyHandler_List_NoTenantReturns401(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	req := httptest.NewRequest(http.MethodGet, "/api/discrepancies", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestDiscrepancyHandler_List_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantA := seedHandlerTenant(t, pool, "list-iso-a")
	tenantB := seedHandlerTenant(t, pool, "list-iso-b")
	t.Cleanup(func() {
		cleanupHandlerTenantData(t, pool, tenantA)
		cleanupHandlerTenantData(t, pool, tenantB)
	})

	seedHandlerDiscrepancy(t, pool, tenantA, "ext-iso-a",
		domain.SeverityHigh, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantB, "ext-iso-b",
		domain.SeverityLow, domain.StatusOpen)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantA.String())
	req := httptest.NewRequest(http.MethodGet, "/api/discrepancies", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	discs := body["discrepancies"].([]interface{})
	assert.Len(t, discs, 1,
		"tenant A should only see its own discrepancies")
}

package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// ---------- GetStats Tests ----------

func TestStatsHandler_GetStats_ReturnsCounts(t *testing.T) {
	pool := newTestPool(t)
	h := handlers.NewStatsHandler(pool)

	tenantID := seedHandlerTenant(t, pool, "stats-tenant")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// Seed discrepancies with different statuses and severities
	seedHandlerDiscrepancy(t, pool, tenantID, "ext-st-001", domain.SeverityHigh, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantID, "ext-st-002", domain.SeverityHigh, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantID, "ext-st-003", domain.SeverityLow, domain.StatusAcknowledged)
	seedHandlerDiscrepancy(t, pool, tenantID, "ext-st-004", domain.SeverityCritical, domain.StatusEscalated)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.GetStats(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	// Check total
	assert.Equal(t, float64(4), body["total"])

	// Check by_status
	byStatus, ok := body["by_status"].(map[string]interface{})
	require.True(t, ok, "by_status must be an object")
	assert.Equal(t, float64(2), byStatus["open"])
	assert.Equal(t, float64(1), byStatus["acknowledged"])
	assert.Equal(t, float64(1), byStatus["escalated"])

	// Check by_severity
	bySeverity, ok := body["by_severity"].(map[string]interface{})
	require.True(t, ok, "by_severity must be an object")
	assert.Equal(t, float64(2), bySeverity["high"])
	assert.Equal(t, float64(1), bySeverity["low"])
	assert.Equal(t, float64(1), bySeverity["critical"])
}

func TestStatsHandler_GetStats_EmptyReturnsZeroes(t *testing.T) {
	pool := newTestPool(t)
	h := handlers.NewStatsHandler(pool)

	tenantID := seedHandlerTenant(t, pool, "stats-empty")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.GetStats(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, float64(0), body["total"])

	byStatus, ok := body["by_status"].(map[string]interface{})
	require.True(t, ok, "by_status must be an object even if empty")
	assert.Empty(t, byStatus)

	bySeverity, ok := body["by_severity"].(map[string]interface{})
	require.True(t, ok, "by_severity must be an object even if empty")
	assert.Empty(t, bySeverity)
}

func TestStatsHandler_GetStats_NoTenantReturns401(t *testing.T) {
	pool := newTestPool(t)
	h := handlers.NewStatsHandler(pool)

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	rec := httptest.NewRecorder()

	h.GetStats(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestStatsHandler_GetStats_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	h := handlers.NewStatsHandler(pool)

	tenantA := seedHandlerTenant(t, pool, "stats-iso-a")
	tenantB := seedHandlerTenant(t, pool, "stats-iso-b")
	t.Cleanup(func() {
		cleanupHandlerTenantData(t, pool, tenantA)
		cleanupHandlerTenantData(t, pool, tenantB)
	})

	seedHandlerDiscrepancy(t, pool, tenantA, "ext-si-a1", domain.SeverityHigh, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantB, "ext-si-b1", domain.SeverityLow, domain.StatusOpen)
	seedHandlerDiscrepancy(t, pool, tenantB, "ext-si-b2", domain.SeverityLow, domain.StatusOpen)

	// Query as tenant A
	reqCtx := ctxutil.SetTenantID(context.Background(), tenantA.String())
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.GetStats(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, float64(1), body["total"],
		"tenant A should only see its own discrepancies")
}

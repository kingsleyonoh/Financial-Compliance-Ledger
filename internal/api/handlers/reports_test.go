package handlers_test

import (
	"bytes"
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
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// ---------- POST /api/reports ----------

func TestReportsHandler_Create_Success(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil) // nil generator — generation starts async

	tenantID := seedHandlerTenant(t, pool, "report-create")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{
		"report_type": domain.ReportTypeDailySummary,
		"date_from":   "2024-01-01",
		"date_to":     "2024-01-31",
	}
	jsonBody, _ := json.Marshal(body)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(jsonBody))
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	report, ok := resp["report"].(map[string]interface{})
	require.True(t, ok, "response must contain 'report' object")
	assert.Equal(t, domain.ReportStatusPending, report["status"])
	assert.Equal(t, domain.ReportTypeDailySummary, report["report_type"])
	assert.NotEmpty(t, report["id"])
}

func TestReportsHandler_Create_MissingReportType_Returns400(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-missing-type")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{
		"date_from": "2024-01-01",
		"date_to":   "2024-01-31",
	}
	jsonBody, _ := json.Marshal(body)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(jsonBody))
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReportsHandler_Create_InvalidReportType_Returns400(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-invalid-type")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{
		"report_type": "invalid_type",
		"date_from":   "2024-01-01",
		"date_to":     "2024-01-31",
	}
	jsonBody, _ := json.Marshal(body)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(jsonBody))
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReportsHandler_Create_MissingDates_Returns400(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-missing-dates")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{
		"report_type": domain.ReportTypeDailySummary,
	}
	jsonBody, _ := json.Marshal(body)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(jsonBody))
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReportsHandler_Create_NoTenant_Returns401(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	body := map[string]interface{}{
		"report_type": domain.ReportTypeDailySummary,
		"date_from":   "2024-01-01",
		"date_to":     "2024-01-31",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(jsonBody))
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestReportsHandler_Create_InvalidBody_Returns400(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-bad-body")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader([]byte("not json")))
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------- GET /api/reports ----------

func TestReportsHandler_List_ReturnsReports(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-list")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// Seed a report via store directly
	report := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Test Daily Summary",
		Parameters: map[string]interface{}{
			"date_from": "2024-01-01",
			"date_to":   "2024-01-31",
		},
		Status: domain.ReportStatusPending,
	}
	_, err := rs.Create(context.Background(), tenantID, report)
	require.NoError(t, err)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	reports, ok := resp["reports"].([]interface{})
	require.True(t, ok, "response must contain 'reports' array")
	assert.Len(t, reports, 1)
}

func TestReportsHandler_List_EmptyReturnsEmptyArray(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-list-empty")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	reports, ok := resp["reports"].([]interface{})
	require.True(t, ok, "response must contain 'reports' array")
	assert.Len(t, reports, 0)
}

func TestReportsHandler_List_NoTenant_Returns401(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestReportsHandler_List_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantA := seedHandlerTenant(t, pool, "report-iso-a")
	tenantB := seedHandlerTenant(t, pool, "report-iso-b")
	t.Cleanup(func() {
		cleanupHandlerTenantData(t, pool, tenantA)
		cleanupHandlerTenantData(t, pool, tenantB)
	})

	// Seed report for tenant B
	_, err := rs.Create(context.Background(), tenantB, &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "B's Report",
		Parameters: map[string]interface{}{"date_from": "2024-01-01", "date_to": "2024-01-31"},
		Status:     domain.ReportStatusPending,
	})
	require.NoError(t, err)

	// Query as tenant A — should see nothing
	reqCtx := ctxutil.SetTenantID(context.Background(), tenantA.String())
	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	reports, ok := resp["reports"].([]interface{})
	require.True(t, ok)
	assert.Len(t, reports, 0, "tenant A must not see tenant B's reports")
}

func TestReportsHandler_Create_AllValidTypes(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-all-types")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	validTypes := []string{
		domain.ReportTypeDailySummary,
		domain.ReportTypeMonthlyAudit,
		domain.ReportTypeDiscrepancyDetail,
		domain.ReportTypeCustom,
	}

	for _, rt := range validTypes {
		body := map[string]interface{}{
			"report_type": rt,
			"date_from":   "2024-01-01",
			"date_to":     "2024-01-31",
		}
		jsonBody, _ := json.Marshal(body)

		reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
		req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(jsonBody))
		req = req.WithContext(reqCtx)
		rec := httptest.NewRecorder()

		h.Create(rec, req)

		assert.Equal(t, http.StatusAccepted, rec.Code, "report type %s should be accepted", rt)
	}
}

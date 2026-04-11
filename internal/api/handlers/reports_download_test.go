package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// ---------- GET /api/reports/:id/download ----------

func TestReportsHandler_Download_Success_HTML(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)

	// Create a temp dir with a test file
	tmpDir := t.TempDir()
	h := handlers.NewReportsHandler(rs, nil)
	h.SetStoragePath(tmpDir)

	tenantID := seedHandlerTenant(t, pool, "report-dl-html")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// Create a report with status "completed" and a real file
	rpt := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Test Download",
		Parameters: map[string]interface{}{"date_from": "2024-01-01", "date_to": "2024-01-31"},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(context.Background(), tenantID, rpt)
	require.NoError(t, err)

	// Write a file at the expected path
	filePath := filepath.Join(tmpDir, tenantID.String(), created.ID+".html")
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, []byte("<html>report</html>"), 0o644))

	// Update report to completed with file path
	reportID, _ := uuid.Parse(created.ID)
	fileSize := int64(len("<html>report</html>"))
	err = rs.UpdateStatus(context.Background(), tenantID, reportID,
		domain.ReportStatusCompleted, &filePath, &fileSize)
	require.NoError(t, err)

	// Build request with chi URL param
	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/"+created.ID+"/download", nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Download(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, rec.Body.String(), "<html>report</html>")
}

func TestReportsHandler_Download_Success_PDF(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)

	tmpDir := t.TempDir()
	h := handlers.NewReportsHandler(rs, nil)
	h.SetStoragePath(tmpDir)

	tenantID := seedHandlerTenant(t, pool, "report-dl-pdf")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	rpt := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Test PDF Download",
		Parameters: map[string]interface{}{"date_from": "2024-01-01", "date_to": "2024-01-31"},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(context.Background(), tenantID, rpt)
	require.NoError(t, err)

	filePath := filepath.Join(tmpDir, tenantID.String(), created.ID+".pdf")
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, []byte("%PDF-1.4 fake"), 0o644))

	reportID, _ := uuid.Parse(created.ID)
	fileSize := int64(len("%PDF-1.4 fake"))
	err = rs.UpdateStatus(context.Background(), tenantID, reportID,
		domain.ReportStatusCompleted, &filePath, &fileSize)
	require.NoError(t, err)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/"+created.ID+"/download", nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Download(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
}

func TestReportsHandler_Download_NotFound_Returns404(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-dl-404")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	fakeID := uuid.New().String()
	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/"+fakeID+"/download", nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", fakeID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Download(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	errObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestReportsHandler_Download_NotCompleted_Returns400(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-dl-pend")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	rpt := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Pending Report",
		Parameters: map[string]interface{}{"date_from": "2024-01-01", "date_to": "2024-01-31"},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(context.Background(), tenantID, rpt)
	require.NoError(t, err)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/"+created.ID+"/download", nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Download(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]interface{}
	err = json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	errObj := resp["error"].(map[string]interface{})
	assert.Contains(t, errObj["message"], "not ready")
}

func TestReportsHandler_Download_FileMissing_Returns500(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-dl-miss")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	rpt := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Missing File Report",
		Parameters: map[string]interface{}{"date_from": "2024-01-01", "date_to": "2024-01-31"},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(context.Background(), tenantID, rpt)
	require.NoError(t, err)

	reportID, _ := uuid.Parse(created.ID)
	fakePath := "/nonexistent/path/report.pdf"
	fileSize := int64(100)
	err = rs.UpdateStatus(context.Background(), tenantID, reportID,
		domain.ReportStatusCompleted, &fakePath, &fileSize)
	require.NoError(t, err)

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/"+created.ID+"/download", nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Download(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestReportsHandler_Download_NoTenant_Returns401(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/"+uuid.New().String()+"/download", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Download(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestReportsHandler_Download_InvalidID_Returns400(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	h := handlers.NewReportsHandler(rs, nil)

	tenantID := seedHandlerTenant(t, pool, "report-dl-badid")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/not-a-uuid/download", nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Download(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReportsHandler_Download_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)

	tmpDir := t.TempDir()
	h := handlers.NewReportsHandler(rs, nil)
	h.SetStoragePath(tmpDir)

	tenantA := seedHandlerTenant(t, pool, "report-dl-iso-a")
	tenantB := seedHandlerTenant(t, pool, "report-dl-iso-b")
	t.Cleanup(func() {
		cleanupHandlerTenantData(t, pool, tenantA)
		cleanupHandlerTenantData(t, pool, tenantB)
	})

	// Create report for tenant B
	rpt := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "B's Report",
		Parameters: map[string]interface{}{"date_from": "2024-01-01", "date_to": "2024-01-31"},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(context.Background(), tenantB, rpt)
	require.NoError(t, err)

	filePath := filepath.Join(tmpDir, tenantB.String(), created.ID+".html")
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, []byte("<html>B</html>"), 0o644))

	reportID, _ := uuid.Parse(created.ID)
	fileSize := int64(14)
	err = rs.UpdateStatus(context.Background(), tenantB, reportID,
		domain.ReportStatusCompleted, &filePath, &fileSize)
	require.NoError(t, err)

	// Try to download as tenant A — should get 404 (tenant scoping)
	reqCtx := ctxutil.SetTenantID(context.Background(), tenantA.String())
	req := httptest.NewRequest(http.MethodGet,
		"/api/reports/"+created.ID+"/download", nil)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Download(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"tenant A must not access tenant B's report")
}

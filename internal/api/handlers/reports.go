package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/report"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// ReportsHandler provides HTTP handlers for report endpoints.
type ReportsHandler struct {
	reportStore     *store.ReportStore
	reportGenerator *report.ReportGenerator
	storagePath     string
}

// NewReportsHandler creates a new ReportsHandler.
func NewReportsHandler(
	rs *store.ReportStore, rg *report.ReportGenerator,
) *ReportsHandler {
	return &ReportsHandler{
		reportStore:     rs,
		reportGenerator: rg,
	}
}

// createReportRequest is the JSON body for POST /api/reports.
type createReportRequest struct {
	ReportType string                 `json:"report_type"`
	DateFrom   string                 `json:"date_from"`
	DateTo     string                 `json:"date_to"`
	Title      string                 `json:"title"`
	Filters    map[string]interface{} `json:"filters"`
}

// Create handles POST /api/reports. It validates the request, creates a
// report record with status "pending", starts generation in a background
// goroutine, and returns 202 Accepted immediately.
func (h *ReportsHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := ctxutil.GetTenantID(r.Context())
	if tenantID == "" {
		RespondError(w, http.StatusUnauthorized,
			"MISSING_TENANT", "Tenant ID not found in context")
		return
	}

	tid, err := uuid.Parse(tenantID)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_TENANT", "Invalid tenant ID format")
		return
	}

	var req createReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_BODY", "Invalid JSON request body")
		return
	}

	if err := validateCreateReport(&req); err != nil {
		RespondError(w, http.StatusBadRequest,
			"VALIDATION_ERROR", err.Error())
		return
	}

	// Build title if not provided
	title := req.Title
	if title == "" {
		title = fmt.Sprintf("%s: %s to %s", req.ReportType, req.DateFrom, req.DateTo)
	}

	// Build parameters map
	params := map[string]interface{}{
		"date_from": req.DateFrom,
		"date_to":   req.DateTo,
	}
	if req.Filters != nil {
		params["filters"] = req.Filters
	}

	rpt := &domain.Report{
		ReportType: req.ReportType,
		Title:      title,
		Parameters: params,
		Status:     domain.ReportStatusPending,
	}

	created, err := h.reportStore.Create(r.Context(), tid, rpt)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"CREATE_FAILED", "Failed to create report")
		return
	}

	// Start background generation if generator is available
	if h.reportGenerator != nil {
		// Use background context — r.Context() is cancelled when the
		// HTTP response is sent, which would kill the goroutine.
		go h.reportGenerator.Generate(context.Background(), tid, created) //nolint:errcheck
	}

	RespondJSON(w, http.StatusAccepted, map[string]interface{}{
		"report": created,
	})
}

// List handles GET /api/reports. Returns all reports for the tenant.
func (h *ReportsHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := ctxutil.GetTenantID(r.Context())
	if tenantID == "" {
		RespondError(w, http.StatusUnauthorized,
			"MISSING_TENANT", "Tenant ID not found in context")
		return
	}

	tid, err := uuid.Parse(tenantID)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_TENANT", "Invalid tenant ID format")
		return
	}

	reports, err := h.reportStore.List(r.Context(), tid)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"LIST_FAILED", "Failed to list reports")
		return
	}

	if reports == nil {
		reports = make([]*domain.Report, 0)
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"reports": reports,
	})
}

// validateCreateReport checks that required fields are present and valid.
func validateCreateReport(req *createReportRequest) error {
	if req.ReportType == "" {
		return fmt.Errorf("report_type is required")
	}
	if !domain.ValidReportType(req.ReportType) {
		return fmt.Errorf("invalid report_type: %s", req.ReportType)
	}
	if req.DateFrom == "" {
		return fmt.Errorf("date_from is required")
	}
	if req.DateTo == "" {
		return fmt.Errorf("date_to is required")
	}
	return nil
}

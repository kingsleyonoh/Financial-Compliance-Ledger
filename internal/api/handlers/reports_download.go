package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// SetStoragePath sets the base storage path for report files.
// Used in tests to point at a temporary directory.
func (h *ReportsHandler) SetStoragePath(path string) {
	h.storagePath = path
}

// Download handles GET /api/reports/{id}/download. It validates the
// report exists, belongs to the requesting tenant, is completed, and
// serves the file with appropriate headers.
func (h *ReportsHandler) Download(w http.ResponseWriter, r *http.Request) {
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

	idStr := chi.URLParam(r, "id")
	reportID, err := uuid.Parse(idStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_ID", "Invalid report ID format")
		return
	}

	rpt, err := h.reportStore.GetByID(r.Context(), tid, reportID)
	if err != nil {
		RespondError(w, http.StatusNotFound,
			"NOT_FOUND", "Report not found")
		return
	}

	if rpt.Status != domain.ReportStatusCompleted {
		RespondError(w, http.StatusBadRequest,
			"NOT_READY", "Report is not ready for download")
		return
	}

	filePath := ""
	if rpt.FilePath != nil {
		filePath = *rpt.FilePath
	}
	if filePath == "" {
		RespondError(w, http.StatusInternalServerError,
			"FILE_MISSING", "Report file path is not set")
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		RespondError(w, http.StatusInternalServerError,
			"FILE_MISSING", "Report file missing")
		return
	}

	// Determine content type from file extension
	contentType := "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".pdf":
		contentType = "application/pdf"
	case ".html", ".htm":
		contentType = "text/html"
	}

	filename := filepath.Base(filePath)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition",
		"attachment; filename=\""+filename+"\"")

	http.ServeFile(w, r, filePath)
}

package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// DiscrepancyHandler provides HTTP handlers for discrepancy endpoints.
type DiscrepancyHandler struct {
	discrepancyStore *store.DiscrepancyStore
	eventStore       *store.EventStore
}

// NewDiscrepancyHandler creates a new DiscrepancyHandler.
func NewDiscrepancyHandler(
	ds *store.DiscrepancyStore, es *store.EventStore,
) *DiscrepancyHandler {
	return &DiscrepancyHandler{
		discrepancyStore: ds,
		eventStore:       es,
	}
}

// List returns a filtered, paginated list of discrepancies for the tenant.
// Query params: status, severity, discrepancy_type, date_from, date_to,
// source_system, cursor, limit (default 25, max 100).
func (h *DiscrepancyHandler) List(w http.ResponseWriter, r *http.Request) {
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

	filters := parseListFilters(r)

	discrepancies, total, err := h.discrepancyStore.List(r.Context(), tid, filters)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"LIST_FAILED", "Failed to list discrepancies")
		return
	}

	// Ensure we return an empty array, not null
	if discrepancies == nil {
		discrepancies = make([]*domain.Discrepancy, 0)
	}

	// Build cursor for next page
	var cursor string
	if len(discrepancies) > 0 && len(discrepancies) == filters.Limit {
		cursor = discrepancies[len(discrepancies)-1].ID
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"discrepancies": discrepancies,
		"total":         total,
		"cursor":        cursor,
	})
}

// GetByID returns a single discrepancy with its event timeline.
func (h *DiscrepancyHandler) GetByID(w http.ResponseWriter, r *http.Request) {
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
	discID, err := uuid.Parse(idStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_ID", "Invalid discrepancy ID format")
		return
	}

	disc, err := h.discrepancyStore.GetByID(r.Context(), tid, discID)
	if err != nil {
		RespondError(w, http.StatusNotFound,
			"NOT_FOUND", "Discrepancy not found")
		return
	}

	events, err := h.eventStore.ListByDiscrepancy(r.Context(), tid, discID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"EVENTS_FAILED", "Failed to retrieve events")
		return
	}

	if events == nil {
		events = make([]*domain.LedgerEvent, 0)
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"discrepancy": disc,
		"events":      events,
	})
}

// parseListFilters extracts filter params from the query string.
func parseListFilters(r *http.Request) store.ListFilters {
	q := r.URL.Query()

	limit := 25
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}

	filters := store.ListFilters{
		Status:          q.Get("status"),
		Severity:        q.Get("severity"),
		DiscrepancyType: q.Get("discrepancy_type"),
		SourceSystem:    q.Get("source_system"),
		Cursor:          q.Get("cursor"),
		Limit:           limit,
	}

	return filters
}

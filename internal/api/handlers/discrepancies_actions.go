package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// Acknowledge transitions a discrepancy from open to acknowledged.
func (h *DiscrepancyHandler) Acknowledge(
	w http.ResponseWriter, r *http.Request,
) {
	tid, discID, ok := parseTenantAndDiscrepancy(w, r)
	if !ok {
		return
	}
	req, ok := decodeWorkflowRequest(w, r)
	if !ok {
		return
	}
	if req.Actor == "" {
		RespondError(w, http.StatusBadRequest,
			"MISSING_ACTOR", "actor field is required")
		return
	}
	if !h.checkOptimisticLock(w, r, tid, discID, req) {
		return
	}

	payload := map[string]interface{}{"previous_status": "open"}
	result, err := h.executeTransition(
		r.Context(), tid, discID,
		domain.StatusAcknowledged, nil,
		domain.EventAcknowledged, req.Actor, payload,
	)
	if err != nil {
		respondTransitionError(w, err, "acknowledge")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"discrepancy": result.disc,
		"event":       result.event,
	})
}

// Investigate transitions a discrepancy from acknowledged to investigating.
func (h *DiscrepancyHandler) Investigate(
	w http.ResponseWriter, r *http.Request,
) {
	tid, discID, ok := parseTenantAndDiscrepancy(w, r)
	if !ok {
		return
	}
	req, ok := decodeWorkflowRequest(w, r)
	if !ok {
		return
	}
	if req.Actor == "" {
		RespondError(w, http.StatusBadRequest,
			"MISSING_ACTOR", "actor field is required")
		return
	}
	if !h.checkOptimisticLock(w, r, tid, discID, req) {
		return
	}

	payload := map[string]interface{}{
		"previous_status": domain.StatusAcknowledged,
	}
	if req.Notes != "" {
		payload["notes"] = req.Notes
	}

	result, err := h.executeTransition(
		r.Context(), tid, discID,
		domain.StatusInvestigating, nil,
		domain.EventInvestigationStarted, req.Actor, payload,
	)
	if err != nil {
		respondTransitionError(w, err, "investigate")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"discrepancy": result.disc,
		"event":       result.event,
	})
}

// Resolve transitions a discrepancy to resolved with a resolution type.
func (h *DiscrepancyHandler) Resolve(
	w http.ResponseWriter, r *http.Request,
) {
	tid, discID, ok := parseTenantAndDiscrepancy(w, r)
	if !ok {
		return
	}
	req, ok := decodeWorkflowRequest(w, r)
	if !ok {
		return
	}
	if req.Actor == "" {
		RespondError(w, http.StatusBadRequest,
			"MISSING_ACTOR", "actor field is required")
		return
	}
	if req.ResolutionType == "" {
		RespondError(w, http.StatusBadRequest,
			"MISSING_RESOLUTION_TYPE", "resolution_type field is required")
		return
	}
	if !domain.ValidResolutionType(req.ResolutionType) {
		RespondError(w, http.StatusBadRequest,
			"INVALID_RESOLUTION_TYPE",
			fmt.Sprintf("Invalid resolution_type: %s. Must be one of: "+
				"match_found, false_positive, manual_adjustment, write_off",
				req.ResolutionType))
		return
	}
	if !h.checkOptimisticLock(w, r, tid, discID, req) {
		return
	}

	now := time.Now()
	payload := map[string]interface{}{
		"resolution_type": req.ResolutionType,
	}
	if req.Notes != "" {
		payload["notes"] = req.Notes
	}

	result, err := h.executeTransition(
		r.Context(), tid, discID,
		domain.StatusResolved, &now,
		domain.EventResolved, req.Actor, payload,
	)
	if err != nil {
		respondTransitionError(w, err, "resolve")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"discrepancy": result.disc,
		"event":       result.event,
	})
}

// AddNote appends a note event to a discrepancy without changing its status.
func (h *DiscrepancyHandler) AddNote(
	w http.ResponseWriter, r *http.Request,
) {
	tid, discID, ok := parseTenantAndDiscrepancy(w, r)
	if !ok {
		return
	}
	req, ok := decodeWorkflowRequest(w, r)
	if !ok {
		return
	}
	if req.Actor == "" {
		RespondError(w, http.StatusBadRequest,
			"MISSING_ACTOR", "actor field is required")
		return
	}
	if req.Content == "" {
		RespondError(w, http.StatusBadRequest,
			"MISSING_CONTENT", "content field is required")
		return
	}
	if !h.checkOptimisticLock(w, r, tid, discID, req) {
		return
	}

	ctx := r.Context()

	// Verify discrepancy exists (tenant-scoped)
	_, err := h.discrepancyStore.GetByID(ctx, tid, discID)
	if err != nil {
		RespondError(w, http.StatusNotFound,
			"NOT_FOUND", "Discrepancy not found")
		return
	}

	event := domain.NewLedgerEvent(
		tid.String(), discID.String(),
		domain.EventNoteAdded, req.Actor, domain.ActorUser,
		map[string]interface{}{"content": req.Content},
	)
	savedEvent, err := h.eventStore.Append(ctx, tid, event)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"EVENT_FAILED", "Failed to append event")
		return
	}

	RespondJSON(w, http.StatusCreated, map[string]interface{}{
		"event": savedEvent,
	})
}

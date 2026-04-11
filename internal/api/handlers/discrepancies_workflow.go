package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// workflowRequest is the common request body for workflow actions.
type workflowRequest struct {
	Actor            string   `json:"actor"`
	Notes            string   `json:"notes"`
	Content          string   `json:"content"`
	ResolutionType   string   `json:"resolution_type"`
	ExpectedSequence *float64 `json:"expected_sequence"`
}

// transitionResult holds the outputs from a successful state transition.
type transitionResult struct {
	disc  *domain.Discrepancy
	event *domain.LedgerEvent
}

// parseTenantAndDiscrepancy extracts and validates tenant_id and
// discrepancy ID from the request context and URL params.
func parseTenantAndDiscrepancy(
	w http.ResponseWriter, r *http.Request,
) (uuid.UUID, uuid.UUID, bool) {
	tenantID := ctxutil.GetTenantID(r.Context())
	if tenantID == "" {
		RespondError(w, http.StatusUnauthorized,
			"MISSING_TENANT", "Tenant ID not found in context")
		return uuid.UUID{}, uuid.UUID{}, false
	}
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_TENANT", "Invalid tenant ID format")
		return uuid.UUID{}, uuid.UUID{}, false
	}
	idStr := chi.URLParam(r, "id")
	discID, err := uuid.Parse(idStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_ID", "Invalid discrepancy ID format")
		return uuid.UUID{}, uuid.UUID{}, false
	}
	return tid, discID, true
}

// decodeWorkflowRequest decodes the JSON body into a workflowRequest.
func decodeWorkflowRequest(
	w http.ResponseWriter, r *http.Request,
) (*workflowRequest, bool) {
	var req workflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_BODY", "Invalid JSON request body")
		return nil, false
	}
	return &req, true
}

// checkOptimisticLock validates expected_sequence against the current
// latest sequence. Returns false if conflict (error already written).
func (h *DiscrepancyHandler) checkOptimisticLock(
	w http.ResponseWriter, r *http.Request,
	tid uuid.UUID, discID uuid.UUID, req *workflowRequest,
) bool {
	if req.ExpectedSequence == nil {
		return true
	}
	expected := int64(*req.ExpectedSequence)
	current, err := h.eventStore.GetLatestSequence(r.Context(), tid, discID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"SEQUENCE_CHECK_FAILED", "Failed to check sequence")
		return false
	}
	if current != expected {
		RespondError(w, http.StatusConflict, "SEQUENCE_CONFLICT",
			fmt.Sprintf("Discrepancy was modified. Expected sequence %d, current is %d",
				expected, current))
		return false
	}
	return true
}

// executeTransition performs the common transactional pattern: fetch
// discrepancy, validate transition, update status, append event, commit.
func (h *DiscrepancyHandler) executeTransition(
	ctx context.Context, tid, discID uuid.UUID,
	targetStatus string, resolvedAt *time.Time,
	eventType, actor string, payload map[string]interface{},
) (*transitionResult, error) {
	pool := h.discrepancyStore.Pool()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("TX_FAILED: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	disc, err := store.GetByIDWith(ctx, tx, tid, discID)
	if err != nil {
		return nil, fmt.Errorf("NOT_FOUND: %w", err)
	}

	if !domain.ValidTransition(disc.Status, targetStatus) {
		return nil, fmt.Errorf("INVALID_TRANSITION: cannot transition "+
			"from %s to %s", disc.Status, targetStatus)
	}

	if err := store.UpdateStatusWith(
		ctx, tx, tid, discID, targetStatus, resolvedAt,
	); err != nil {
		return nil, fmt.Errorf("UPDATE_FAILED: %w", err)
	}
	disc.Status = targetStatus
	if resolvedAt != nil {
		disc.ResolvedAt = resolvedAt
	}

	event := domain.NewLedgerEvent(
		tid.String(), discID.String(),
		eventType, actor, domain.ActorUser, payload,
	)
	savedEvent, err := store.AppendWith(ctx, tx, tid, event)
	if err != nil {
		return nil, fmt.Errorf("EVENT_FAILED: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("COMMIT_FAILED: %w", err)
	}

	return &transitionResult{disc: disc, event: savedEvent}, nil
}

// respondTransitionError maps executeTransition errors to HTTP responses.
func respondTransitionError(
	w http.ResponseWriter, err error, action string,
) {
	msg := err.Error()
	switch {
	case len(msg) >= 9 && msg[:9] == "NOT_FOUND":
		RespondError(w, http.StatusNotFound,
			"NOT_FOUND", "Discrepancy not found")
	case len(msg) >= 18 && msg[:18] == "INVALID_TRANSITION":
		RespondError(w, http.StatusConflict,
			"INVALID_TRANSITION", fmt.Sprintf("Cannot %s: %s", action, msg[20:]))
	default:
		RespondError(w, http.StatusInternalServerError,
			"INTERNAL_ERROR", fmt.Sprintf("Failed to %s", action))
	}
}

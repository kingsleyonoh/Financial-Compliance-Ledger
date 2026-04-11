package handlers_test

import (
	"bytes"
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

// ---------- Helper: build workflow request ----------

func buildWorkflowRequest(
	t *testing.T, tenantID string, discID string,
	body interface{},
) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/discrepancies/%s/action", discID), &buf)
	req.Header.Set("Content-Type", "application/json")

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID)
	req = req.WithContext(reqCtx)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", discID)
	req = req.WithContext(context.WithValue(
		req.Context(), chi.RouteCtxKey, rctx))
	return req
}

// ========== Acknowledge Tests ==========

func TestAcknowledge_Success(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "ack-success")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-ack-001", domain.SeverityHigh, domain.StatusOpen)

	body := map[string]interface{}{"actor": "user@company.com"}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	disc := resp["discrepancy"].(map[string]interface{})
	assert.Equal(t, domain.StatusAcknowledged, disc["status"])

	evt := resp["event"].(map[string]interface{})
	assert.Equal(t, domain.EventAcknowledged, evt["event_type"])
	assert.Equal(t, "user@company.com", evt["actor"])
}

func TestAcknowledge_InvalidTransition_Returns409(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "ack-invalid")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// Create a discrepancy in "resolved" status — cannot acknowledge
	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-ack-inv", domain.SeverityHigh, domain.StatusResolved)

	body := map[string]interface{}{"actor": "user@company.com"}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	errBody := resp["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_TRANSITION", errBody["code"])
}

func TestAcknowledge_MissingActor_Returns400(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "ack-no-actor")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-ack-na", domain.SeverityHigh, domain.StatusOpen)

	body := map[string]interface{}{}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAcknowledge_NotFound_Returns404(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "ack-notfound")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	fakeID := uuid.New()
	body := map[string]interface{}{"actor": "user@company.com"}
	req := buildWorkflowRequest(t, tenantID.String(), fakeID.String(), body)
	rec := httptest.NewRecorder()

	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAcknowledge_NoTenant_Returns401(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	body := map[string]interface{}{"actor": "user@company.com"}
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	req := httptest.NewRequest(http.MethodPost,
		"/api/discrepancies/"+uuid.New().String()+"/acknowledge", &buf)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(
		req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ========== Investigate Tests ==========

func TestInvestigate_Success(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "inv-success")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-inv-001", domain.SeverityHigh, domain.StatusAcknowledged)

	body := map[string]interface{}{
		"actor": "investigator@company.com",
		"notes": "Starting investigation",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Investigate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	disc := resp["discrepancy"].(map[string]interface{})
	assert.Equal(t, domain.StatusInvestigating, disc["status"])

	evt := resp["event"].(map[string]interface{})
	assert.Equal(t, domain.EventInvestigationStarted, evt["event_type"])

	payload := evt["payload"].(map[string]interface{})
	assert.Equal(t, "Starting investigation", payload["notes"])
}

func TestInvestigate_WithoutNotes_Success(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "inv-no-notes")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-inv-nn", domain.SeverityHigh, domain.StatusAcknowledged)

	body := map[string]interface{}{"actor": "user@company.com"}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Investigate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestInvestigate_InvalidTransition_Returns409(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "inv-invalid")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// open -> investigating is NOT allowed; must be acknowledged first
	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-inv-inv", domain.SeverityHigh, domain.StatusOpen)

	body := map[string]interface{}{"actor": "user@company.com"}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Investigate(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestInvestigate_MissingActor_Returns400(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "inv-no-actor")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-inv-na", domain.SeverityHigh, domain.StatusAcknowledged)

	body := map[string]interface{}{}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Investigate(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ========== Resolve Tests ==========

func TestResolve_Success(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "res-success")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-res-001", domain.SeverityHigh, domain.StatusInvestigating)

	body := map[string]interface{}{
		"actor":           "resolver@company.com",
		"resolution_type": "match_found",
		"notes":           "Found matching transaction",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Resolve(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	disc := resp["discrepancy"].(map[string]interface{})
	assert.Equal(t, domain.StatusResolved, disc["status"])
	assert.NotNil(t, disc["resolved_at"],
		"resolved_at must be set on resolution")

	evt := resp["event"].(map[string]interface{})
	assert.Equal(t, domain.EventResolved, evt["event_type"])

	payload := evt["payload"].(map[string]interface{})
	assert.Equal(t, "match_found", payload["resolution_type"])
}

func TestResolve_AllResolutionTypes(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "res-all-types")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	resolutionTypes := []string{
		domain.ResolutionMatchFound,
		domain.ResolutionFalsePositive,
		domain.ResolutionManualAdjustment,
		domain.ResolutionWriteOff,
	}

	for i, rt := range resolutionTypes {
		discID := seedHandlerDiscrepancy(t, pool, tenantID,
			fmt.Sprintf("ext-res-rt-%d", i),
			domain.SeverityHigh, domain.StatusInvestigating)

		body := map[string]interface{}{
			"actor":           "user@company.com",
			"resolution_type": rt,
		}
		req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
		rec := httptest.NewRecorder()

		h.Resolve(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code,
			"resolution_type %s should be accepted", rt)
	}
}

func TestResolve_InvalidResolutionType_Returns400(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "res-bad-type")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-res-bad", domain.SeverityHigh, domain.StatusInvestigating)

	body := map[string]interface{}{
		"actor":           "user@company.com",
		"resolution_type": "invalid_type",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Resolve(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestResolve_MissingResolutionType_Returns400(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "res-no-type")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-res-nt", domain.SeverityHigh, domain.StatusInvestigating)

	body := map[string]interface{}{
		"actor": "user@company.com",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Resolve(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestResolve_InvalidTransition_Returns409(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "res-invalid")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// open -> resolved is NOT allowed
	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-res-inv", domain.SeverityHigh, domain.StatusOpen)

	body := map[string]interface{}{
		"actor":           "user@company.com",
		"resolution_type": "match_found",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Resolve(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestResolve_MissingActor_Returns400(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "res-no-actor")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-res-na", domain.SeverityHigh, domain.StatusInvestigating)

	body := map[string]interface{}{
		"resolution_type": "match_found",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Resolve(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestResolve_FromEscalated_Success(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "res-esc")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// escalated -> resolved is valid
	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-res-esc", domain.SeverityHigh, domain.StatusEscalated)

	body := map[string]interface{}{
		"actor":           "user@company.com",
		"resolution_type": "manual_adjustment",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Resolve(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ========== AddNote Tests ==========

func TestAddNote_Success(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "note-success")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-note-001", domain.SeverityHigh, domain.StatusOpen)

	body := map[string]interface{}{
		"actor":   "user@company.com",
		"content": "This is a note about the discrepancy",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.AddNote(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	evt := resp["event"].(map[string]interface{})
	assert.Equal(t, domain.EventNoteAdded, evt["event_type"])
	assert.Equal(t, "user@company.com", evt["actor"])

	payload := evt["payload"].(map[string]interface{})
	assert.Equal(t, "This is a note about the discrepancy", payload["content"])

	// Verify no discrepancy key in response (notes don't change status)
	_, hasDisc := resp["discrepancy"]
	assert.False(t, hasDisc,
		"AddNote should not return discrepancy (status unchanged)")
}

func TestAddNote_MissingActor_Returns400(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "note-no-actor")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-note-na", domain.SeverityHigh, domain.StatusOpen)

	body := map[string]interface{}{
		"content": "some note",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.AddNote(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAddNote_MissingContent_Returns400(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "note-no-content")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-note-nc", domain.SeverityHigh, domain.StatusOpen)

	body := map[string]interface{}{
		"actor": "user@company.com",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.AddNote(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAddNote_NotFound_Returns404(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "note-notfound")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	fakeID := uuid.New()
	body := map[string]interface{}{
		"actor":   "user@company.com",
		"content": "note for ghost",
	}
	req := buildWorkflowRequest(t, tenantID.String(), fakeID.String(), body)
	rec := httptest.NewRecorder()

	h.AddNote(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAddNote_DoesNotChangeStatus(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "note-status")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-note-st", domain.SeverityHigh, domain.StatusInvestigating)

	body := map[string]interface{}{
		"actor":   "user@company.com",
		"content": "status should stay investigating",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.AddNote(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	// Verify status hasn't changed by reading discrepancy
	ctx := context.Background()
	disc, err := ds.GetByID(ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusInvestigating, disc.Status)
}

// ========== Optimistic Locking Tests ==========

func TestAcknowledge_OptimisticLocking_Success(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "opt-lock-ok")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-opt-ok", domain.SeverityHigh, domain.StatusOpen)

	// Seed a "received" event so we have a known sequence
	seedHandlerEvent(t, pool, tenantID, discID, domain.EventReceived)

	// Get the current sequence
	ctx := context.Background()
	seq, err := es.GetLatestSequence(ctx, tenantID, discID)
	require.NoError(t, err)
	require.Greater(t, seq, int64(0))

	body := map[string]interface{}{
		"actor":             "user@company.com",
		"expected_sequence": seq,
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAcknowledge_OptimisticLocking_Conflict(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "opt-lock-fail")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-opt-fail", domain.SeverityHigh, domain.StatusOpen)

	// Seed a "received" event
	seedHandlerEvent(t, pool, tenantID, discID, domain.EventReceived)

	// Use a wrong expected_sequence (stale)
	body := map[string]interface{}{
		"actor":             "user@company.com",
		"expected_sequence": float64(999),
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	errBody := resp["error"].(map[string]interface{})
	assert.Equal(t, "SEQUENCE_CONFLICT", errBody["code"])
}

func TestAcknowledge_OptimisticLocking_NotProvided_Skips(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "opt-lock-skip")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-opt-skip", domain.SeverityHigh, domain.StatusOpen)

	// No expected_sequence field — should succeed without check
	body := map[string]interface{}{
		"actor": "user@company.com",
	}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ========== Full Workflow Integration Test ==========

func TestFullWorkflow_Open_Ack_Investigate_Resolve(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantID := seedHandlerTenant(t, pool, "full-wf")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-full-wf", domain.SeverityHigh, domain.StatusOpen)

	// Step 1: Acknowledge
	ackBody := map[string]interface{}{"actor": "ack-user@co.com"}
	req := buildWorkflowRequest(t, tenantID.String(), discID.String(), ackBody)
	rec := httptest.NewRecorder()
	h.Acknowledge(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Step 2: Investigate
	invBody := map[string]interface{}{
		"actor": "inv-user@co.com",
		"notes": "Looking into it",
	}
	req = buildWorkflowRequest(t, tenantID.String(), discID.String(), invBody)
	rec = httptest.NewRecorder()
	h.Investigate(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Step 3: Add a note during investigation
	noteBody := map[string]interface{}{
		"actor":   "inv-user@co.com",
		"content": "Found something",
	}
	req = buildWorkflowRequest(t, tenantID.String(), discID.String(), noteBody)
	rec = httptest.NewRecorder()
	h.AddNote(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// Step 4: Resolve
	resBody := map[string]interface{}{
		"actor":           "inv-user@co.com",
		"resolution_type": "match_found",
		"notes":           "Transaction reconciled",
	}
	req = buildWorkflowRequest(t, tenantID.String(), discID.String(), resBody)
	rec = httptest.NewRecorder()
	h.Resolve(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify final state
	ctx := context.Background()
	disc, err := ds.GetByID(ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusResolved, disc.Status)
	assert.NotNil(t, disc.ResolvedAt)

	// Verify all events were recorded (ack + investigation + note + resolved)
	events, err := es.ListByDiscrepancy(ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Len(t, events, 4,
		"should have acknowledged, investigation_started, note_added, and resolved events")
}

// ========== Tenant Isolation for Workflow Actions ==========

func TestAcknowledge_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	h := handlers.NewDiscrepancyHandler(ds, es)

	tenantA := seedHandlerTenant(t, pool, "ack-iso-a")
	tenantB := seedHandlerTenant(t, pool, "ack-iso-b")
	t.Cleanup(func() {
		cleanupHandlerTenantData(t, pool, tenantA)
		cleanupHandlerTenantData(t, pool, tenantB)
	})

	// Create discrepancy in tenant B
	discID := seedHandlerDiscrepancy(t, pool, tenantB,
		"ext-ack-iso", domain.SeverityHigh, domain.StatusOpen)

	// Try to acknowledge as tenant A — should 404
	body := map[string]interface{}{"actor": "user@a.com"}
	req := buildWorkflowRequest(t, tenantA.String(), discID.String(), body)
	rec := httptest.NewRecorder()

	h.Acknowledge(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"tenant A must not access tenant B's discrepancy")
}

// ========== GetLatestSequence Tests ==========

func TestEventStore_GetLatestSequence(t *testing.T) {
	pool := newTestPool(t)
	es := store.NewEventStore(pool)

	tenantID := seedHandlerTenant(t, pool, "seq-test")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	discID := seedHandlerDiscrepancy(t, pool, tenantID,
		"ext-seq-001", domain.SeverityHigh, domain.StatusOpen)

	ctx := context.Background()

	// No events yet — should return 0
	seq, err := es.GetLatestSequence(ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), seq)

	// Add an event
	seedHandlerEvent(t, pool, tenantID, discID, domain.EventReceived)

	seq, err = es.GetLatestSequence(ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Greater(t, seq, int64(0))

	// Add another event — sequence should increase
	seedHandlerEvent(t, pool, tenantID, discID, domain.EventAcknowledged)

	seq2, err := es.GetLatestSequence(ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Greater(t, seq2, seq)
}

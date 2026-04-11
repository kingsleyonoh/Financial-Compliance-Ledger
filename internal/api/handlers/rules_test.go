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

// ---------- Helper: build rule request ----------

func buildRuleRequest(
	t *testing.T, method, path, tenantID string, body interface{},
) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")

	if tenantID != "" {
		reqCtx := ctxutil.SetTenantID(context.Background(), tenantID)
		req = req.WithContext(reqCtx)
	}
	return req
}

// buildRuleRequestWithID builds a request with :id in chi URL params.
func buildRuleRequestWithID(
	t *testing.T, method, path, tenantID, ruleID string,
	body interface{},
) *http.Request {
	t.Helper()
	req := buildRuleRequest(t, method, path, tenantID, body)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", ruleID)
	req = req.WithContext(context.WithValue(
		req.Context(), chi.RouteCtxKey, rctx))
	return req
}

// seedHandlerRule inserts a test escalation rule and returns its UUID.
func seedHandlerRule(
	t *testing.T, pool interface{ Exec(ctx context.Context, sql string, args ...interface{}) (interface{ RowsAffected() int64 }, error) },
	rs *store.RuleStore, tenantID uuid.UUID,
	name, severityMatch, triggerStatus, action string,
	triggerAfterHrs, priority int, isActive bool,
) string {
	t.Helper()
	ctx := context.Background()
	rule := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            name,
		SeverityMatch:   severityMatch,
		TriggerAfterHrs: triggerAfterHrs,
		TriggerStatus:   triggerStatus,
		Action:          action,
		ActionConfig:    map[string]interface{}{},
		IsActive:        isActive,
		Priority:        priority,
	}
	created, err := rs.Create(ctx, tenantID, rule)
	require.NoError(t, err, "failed to seed rule")
	return created.ID
}

// ========== List Tests ==========

func TestRulesHandler_List_ReturnsRules(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-list")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	seedHandlerRule(t, nil, rs, tenantID, "Rule A", "*",
		domain.StatusOpen, domain.ActionNotify, 4, 1, true)
	seedHandlerRule(t, nil, rs, tenantID, "Rule B",
		domain.SeverityHigh, domain.StatusOpen,
		domain.ActionEscalate, 8, 2, true)

	req := buildRuleRequest(t, http.MethodGet,
		"/api/rules", tenantID.String(), nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	rules, ok := body["rules"].([]interface{})
	require.True(t, ok, "rules must be an array")
	assert.Len(t, rules, 2)
}

func TestRulesHandler_List_EmptyReturnsEmptyArray(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-list-empty")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	req := buildRuleRequest(t, http.MethodGet,
		"/api/rules", tenantID.String(), nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	rules, ok := body["rules"].([]interface{})
	require.True(t, ok, "rules must be an array, not null")
	assert.Len(t, rules, 0)
}

func TestRulesHandler_List_ActiveOnlyFilter(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-list-active")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	seedHandlerRule(t, nil, rs, tenantID, "Active Rule", "*",
		domain.StatusOpen, domain.ActionNotify, 4, 1, true)
	seedHandlerRule(t, nil, rs, tenantID, "Inactive Rule", "*",
		domain.StatusOpen, domain.ActionNotify, 8, 2, false)

	req := buildRuleRequest(t, http.MethodGet,
		"/api/rules?active_only=true", tenantID.String(), nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	rules := body["rules"].([]interface{})
	assert.Len(t, rules, 1)
}

func TestRulesHandler_List_NoTenantReturns401(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	req := buildRuleRequest(t, http.MethodGet, "/api/rules", "", nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRulesHandler_List_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantA := seedHandlerTenant(t, pool, "rules-iso-a")
	tenantB := seedHandlerTenant(t, pool, "rules-iso-b")
	t.Cleanup(func() {
		cleanupHandlerTenantData(t, pool, tenantA)
		cleanupHandlerTenantData(t, pool, tenantB)
	})

	seedHandlerRule(t, nil, rs, tenantA, "A Rule", "*",
		domain.StatusOpen, domain.ActionNotify, 4, 1, true)
	seedHandlerRule(t, nil, rs, tenantB, "B Rule", "*",
		domain.StatusOpen, domain.ActionNotify, 4, 1, true)

	req := buildRuleRequest(t, http.MethodGet,
		"/api/rules", tenantA.String(), nil)
	rec := httptest.NewRecorder()

	h.List(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	rules := body["rules"].([]interface{})
	assert.Len(t, rules, 1, "tenant A should only see its own rules")
}

// ========== Create Tests ==========

func TestRulesHandler_Create_Success(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-create")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{
		"name":             "Critical Alert",
		"severity_match":   "critical",
		"trigger_after_hrs": 4,
		"trigger_status":   "open",
		"action":           "notify",
		"action_config":    map[string]interface{}{"channel": "#alerts"},
		"priority":         10,
	}
	req := buildRuleRequest(t, http.MethodPost,
		"/api/rules", tenantID.String(), body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	rule := resp["rule"].(map[string]interface{})
	assert.Equal(t, "Critical Alert", rule["name"])
	assert.Equal(t, "critical", rule["severity_match"])
	assert.Equal(t, float64(4), rule["trigger_after_hrs"])
	assert.Equal(t, "open", rule["trigger_status"])
	assert.Equal(t, "notify", rule["action"])
	assert.Equal(t, float64(10), rule["priority"])
	assert.NotEmpty(t, rule["id"])
	assert.NotEmpty(t, rule["created_at"])
}

func TestRulesHandler_Create_DefaultsApplied(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-defaults")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// Omit action_config and priority — should default
	body := map[string]interface{}{
		"name":             "Default Rule",
		"severity_match":   "*",
		"trigger_after_hrs": 8,
		"trigger_status":   "open",
		"action":           "auto_close",
	}
	req := buildRuleRequest(t, http.MethodPost,
		"/api/rules", tenantID.String(), body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	rule := resp["rule"].(map[string]interface{})
	assert.Equal(t, float64(0), rule["priority"])

	config, ok := rule["action_config"].(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, config) // should be empty object {}
}

func TestRulesHandler_Create_MissingRequiredField(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-missing")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	// Missing "name"
	body := map[string]interface{}{
		"severity_match":   "high",
		"trigger_after_hrs": 4,
		"trigger_status":   "open",
		"action":           "notify",
	}
	req := buildRuleRequest(t, http.MethodPost,
		"/api/rules", tenantID.String(), body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRulesHandler_Create_InvalidSeverityMatch(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-bad-sev")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{
		"name":             "Bad Severity",
		"severity_match":   "INVALID",
		"trigger_after_hrs": 4,
		"trigger_status":   "open",
		"action":           "notify",
	}
	req := buildRuleRequest(t, http.MethodPost,
		"/api/rules", tenantID.String(), body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRulesHandler_Create_InvalidAction(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-bad-action")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{
		"name":             "Bad Action",
		"severity_match":   "high",
		"trigger_after_hrs": 4,
		"trigger_status":   "open",
		"action":           "INVALID",
	}
	req := buildRuleRequest(t, http.MethodPost,
		"/api/rules", tenantID.String(), body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRulesHandler_Create_InvalidTriggerStatus(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-bad-status")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{
		"name":             "Bad Status",
		"severity_match":   "high",
		"trigger_after_hrs": 4,
		"trigger_status":   "INVALID",
		"action":           "notify",
	}
	req := buildRuleRequest(t, http.MethodPost,
		"/api/rules", tenantID.String(), body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRulesHandler_Create_NoTenantReturns401(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	body := map[string]interface{}{
		"name":             "No Tenant",
		"severity_match":   "high",
		"trigger_after_hrs": 4,
		"trigger_status":   "open",
		"action":           "notify",
	}
	req := buildRuleRequest(t, http.MethodPost, "/api/rules", "", body)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRulesHandler_Create_InvalidJSON(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-bad-json")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	reqCtx := ctxutil.SetTenantID(context.Background(), tenantID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/rules",
		bytes.NewBufferString("not-json{{{"))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()

	h.Create(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ========== Update Tests ==========

func TestRulesHandler_Update_Success(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-update")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	ruleID := seedHandlerRule(t, nil, rs, tenantID, "Update Me", "*",
		domain.StatusOpen, domain.ActionNotify, 4, 1, true)

	body := map[string]interface{}{
		"name":     "Updated Name",
		"priority": 99,
	}
	req := buildRuleRequestWithID(t, http.MethodPut,
		fmt.Sprintf("/api/rules/%s", ruleID),
		tenantID.String(), ruleID, body)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	rule := resp["rule"].(map[string]interface{})
	assert.Equal(t, "Updated Name", rule["name"])
	assert.Equal(t, float64(99), rule["priority"])
	// Unchanged fields
	assert.Equal(t, "*", rule["severity_match"])
	assert.Equal(t, "notify", rule["action"])
}

func TestRulesHandler_Update_NotFound(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-update-nf")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	fakeID := uuid.New().String()
	body := map[string]interface{}{"name": "Ghost Rule"}
	req := buildRuleRequestWithID(t, http.MethodPut,
		fmt.Sprintf("/api/rules/%s", fakeID),
		tenantID.String(), fakeID, body)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRulesHandler_Update_InvalidID(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-update-bad-id")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	body := map[string]interface{}{"name": "Updated"}
	req := buildRuleRequestWithID(t, http.MethodPut,
		"/api/rules/not-a-uuid",
		tenantID.String(), "not-a-uuid", body)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRulesHandler_Update_InvalidSeverityMatch(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-update-bad-sev")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	ruleID := seedHandlerRule(t, nil, rs, tenantID, "Update Sev", "*",
		domain.StatusOpen, domain.ActionNotify, 4, 1, true)

	body := map[string]interface{}{"severity_match": "INVALID"}
	req := buildRuleRequestWithID(t, http.MethodPut,
		fmt.Sprintf("/api/rules/%s", ruleID),
		tenantID.String(), ruleID, body)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRulesHandler_Update_NoTenantReturns401(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	fakeID := uuid.New().String()
	body := map[string]interface{}{"name": "X"}
	req := buildRuleRequestWithID(t, http.MethodPut,
		fmt.Sprintf("/api/rules/%s", fakeID),
		"", fakeID, body)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ========== Delete Tests ==========

func TestRulesHandler_Delete_Success(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-delete")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	ruleID := seedHandlerRule(t, nil, rs, tenantID, "Delete Me", "*",
		domain.StatusOpen, domain.ActionNotify, 4, 1, true)

	req := buildRuleRequestWithID(t, http.MethodDelete,
		fmt.Sprintf("/api/rules/%s", ruleID),
		tenantID.String(), ruleID, nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestRulesHandler_Delete_NotFound(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-delete-nf")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	fakeID := uuid.New().String()
	req := buildRuleRequestWithID(t, http.MethodDelete,
		fmt.Sprintf("/api/rules/%s", fakeID),
		tenantID.String(), fakeID, nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRulesHandler_Delete_InvalidID(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantID := seedHandlerTenant(t, pool, "rules-delete-bad-id")
	t.Cleanup(func() { cleanupHandlerTenantData(t, pool, tenantID) })

	req := buildRuleRequestWithID(t, http.MethodDelete,
		"/api/rules/not-a-uuid",
		tenantID.String(), "not-a-uuid", nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRulesHandler_Delete_NoTenantReturns401(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	fakeID := uuid.New().String()
	req := buildRuleRequestWithID(t, http.MethodDelete,
		fmt.Sprintf("/api/rules/%s", fakeID),
		"", fakeID, nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRulesHandler_Delete_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	h := handlers.NewRulesHandler(rs)

	tenantA := seedHandlerTenant(t, pool, "rules-del-iso-a")
	tenantB := seedHandlerTenant(t, pool, "rules-del-iso-b")
	t.Cleanup(func() {
		cleanupHandlerTenantData(t, pool, tenantA)
		cleanupHandlerTenantData(t, pool, tenantB)
	})

	// Create rule in tenant B
	ruleID := seedHandlerRule(t, nil, rs, tenantB, "B Rule", "*",
		domain.StatusOpen, domain.ActionNotify, 4, 1, true)

	// Try to delete as tenant A — should 404
	req := buildRuleRequestWithID(t, http.MethodDelete,
		fmt.Sprintf("/api/rules/%s", ruleID),
		tenantA.String(), ruleID, nil)
	rec := httptest.NewRecorder()

	h.Delete(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"tenant A must not delete tenant B's rule")
}

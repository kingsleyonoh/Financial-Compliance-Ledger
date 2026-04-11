package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// RulesHandler provides HTTP handlers for escalation rule endpoints.
type RulesHandler struct {
	ruleStore *store.RuleStore
}

// NewRulesHandler creates a new RulesHandler.
func NewRulesHandler(rs *store.RuleStore) *RulesHandler {
	return &RulesHandler{ruleStore: rs}
}

// List returns all escalation rules for the tenant.
// Query params: active_only (bool, default false).
func (h *RulesHandler) List(w http.ResponseWriter, r *http.Request) {
	tid, ok := parseTenantID(w, r)
	if !ok {
		return
	}

	activeOnly := strings.EqualFold(r.URL.Query().Get("active_only"), "true")

	rules, err := h.ruleStore.List(r.Context(), tid, activeOnly)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"LIST_FAILED", "Failed to list rules")
		return
	}

	if rules == nil {
		rules = make([]*domain.EscalationRule, 0)
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"rules": rules,
	})
}

// createRuleRequest is the JSON body for creating an escalation rule.
type createRuleRequest struct {
	Name            string                 `json:"name"`
	SeverityMatch   string                 `json:"severity_match"`
	TriggerAfterHrs *int                   `json:"trigger_after_hrs"`
	TriggerStatus   string                 `json:"trigger_status"`
	Action          string                 `json:"action"`
	ActionConfig    map[string]interface{} `json:"action_config"`
	Priority        *int                   `json:"priority"`
}

// Create creates a new escalation rule.
func (h *RulesHandler) Create(w http.ResponseWriter, r *http.Request) {
	tid, ok := parseTenantID(w, r)
	if !ok {
		return
	}

	var req createRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_BODY", "Invalid JSON request body")
		return
	}

	if err := validateCreateRule(&req); err != "" {
		RespondError(w, http.StatusBadRequest, "VALIDATION_ERROR", err)
		return
	}

	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	if req.ActionConfig == nil {
		req.ActionConfig = map[string]interface{}{}
	}

	rule := &domain.EscalationRule{
		TenantID:        tid.String(),
		Name:            req.Name,
		SeverityMatch:   req.SeverityMatch,
		TriggerAfterHrs: *req.TriggerAfterHrs,
		TriggerStatus:   req.TriggerStatus,
		Action:          req.Action,
		ActionConfig:    req.ActionConfig,
		IsActive:        true,
		Priority:        priority,
	}

	created, err := h.ruleStore.Create(r.Context(), tid, rule)
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"CREATE_FAILED", "Failed to create rule")
		return
	}

	RespondJSON(w, http.StatusCreated, map[string]interface{}{
		"rule": created,
	})
}

// updateRuleRequest is the JSON body for updating an escalation rule.
type updateRuleRequest struct {
	Name            *string                 `json:"name"`
	SeverityMatch   *string                 `json:"severity_match"`
	TriggerAfterHrs *int                    `json:"trigger_after_hrs"`
	TriggerStatus   *string                 `json:"trigger_status"`
	Action          *string                 `json:"action"`
	ActionConfig    *map[string]interface{} `json:"action_config"`
	IsActive        *bool                   `json:"is_active"`
	Priority        *int                    `json:"priority"`
}

// Update performs a partial update on an escalation rule.
func (h *RulesHandler) Update(w http.ResponseWriter, r *http.Request) {
	tid, ok := parseTenantID(w, r)
	if !ok {
		return
	}

	ruleID, err := parseURLParamUUID(w, r, "id")
	if err != nil {
		return
	}

	var req updateRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_BODY", "Invalid JSON request body")
		return
	}

	if valErr := validateUpdateRule(&req); valErr != "" {
		RespondError(w, http.StatusBadRequest,
			"VALIDATION_ERROR", valErr)
		return
	}

	updates := store.RuleUpdate{
		Name:            req.Name,
		SeverityMatch:   req.SeverityMatch,
		TriggerAfterHrs: req.TriggerAfterHrs,
		TriggerStatus:   req.TriggerStatus,
		Action:          req.Action,
		ActionConfig:    req.ActionConfig,
		IsActive:        req.IsActive,
		Priority:        req.Priority,
	}

	updated, err := h.ruleStore.Update(r.Context(), tid, ruleID, updates)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			RespondError(w, http.StatusNotFound,
				"NOT_FOUND", "Rule not found")
			return
		}
		RespondError(w, http.StatusInternalServerError,
			"UPDATE_FAILED", "Failed to update rule")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{
		"rule": updated,
	})
}

// Delete removes an escalation rule.
func (h *RulesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tid, ok := parseTenantID(w, r)
	if !ok {
		return
	}

	ruleID, err := parseURLParamUUID(w, r, "id")
	if err != nil {
		return
	}

	if err := h.ruleStore.Delete(r.Context(), tid, ruleID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			RespondError(w, http.StatusNotFound,
				"NOT_FOUND", "Rule not found")
			return
		}
		RespondError(w, http.StatusInternalServerError,
			"DELETE_FAILED", "Failed to delete rule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseTenantID extracts the tenant ID from the request context.
func parseTenantID(
	w http.ResponseWriter, r *http.Request,
) (uuid.UUID, bool) {
	tenantID := ctxutil.GetTenantID(r.Context())
	if tenantID == "" {
		RespondError(w, http.StatusUnauthorized,
			"MISSING_TENANT", "Tenant ID not found in context")
		return uuid.UUID{}, false
	}
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_TENANT", "Invalid tenant ID format")
		return uuid.UUID{}, false
	}
	return tid, true
}

// parseURLParamUUID extracts a UUID from the URL params. Returns an
// error (already written to w) if the param is not a valid UUID.
func parseURLParamUUID(
	w http.ResponseWriter, r *http.Request, param string,
) (uuid.UUID, error) {
	idStr := chi.URLParam(r, param)
	id, err := uuid.Parse(idStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_ID", "Invalid "+param+" format")
		return uuid.UUID{}, err
	}
	return id, nil
}

// validateCreateRule checks required fields and validates enum values.
// Returns an error message string, or "" if valid.
func validateCreateRule(req *createRuleRequest) string {
	if req.Name == "" {
		return "name is required"
	}
	if req.SeverityMatch == "" {
		return "severity_match is required"
	}
	if !domain.ValidSeverityMatch(req.SeverityMatch) {
		return "invalid severity_match value"
	}
	if req.TriggerAfterHrs == nil {
		return "trigger_after_hrs is required"
	}
	if req.TriggerStatus == "" {
		return "trigger_status is required"
	}
	if !domain.ValidTriggerStatus(req.TriggerStatus) {
		return "invalid trigger_status value"
	}
	if req.Action == "" {
		return "action is required"
	}
	if !domain.ValidAction(req.Action) {
		return "invalid action value"
	}
	return ""
}

// validateUpdateRule checks that any provided enum values are valid.
func validateUpdateRule(req *updateRuleRequest) string {
	if req.SeverityMatch != nil && !domain.ValidSeverityMatch(*req.SeverityMatch) {
		return "invalid severity_match value"
	}
	if req.TriggerStatus != nil && !domain.ValidTriggerStatus(*req.TriggerStatus) {
		return "invalid trigger_status value"
	}
	if req.Action != nil && !domain.ValidAction(*req.Action) {
		return "invalid action value"
	}
	return ""
}

package domain

import "time"

// Action constants represent the actions an escalation rule can trigger.
const (
	ActionNotify    = "notify"
	ActionEscalate  = "escalate"
	ActionAutoClose = "auto_close"
)

// validActions is the set of all allowed escalation actions.
var validActions = map[string]bool{
	ActionNotify:    true,
	ActionEscalate:  true,
	ActionAutoClose: true,
}

// validSeverityMatches is the set of all allowed severity match values.
// Includes "*" wildcard which matches any severity.
var validSeverityMatches = map[string]bool{
	SeverityLow:      true,
	SeverityMedium:   true,
	SeverityHigh:     true,
	SeverityCritical: true,
	"*":              true,
}

// validTriggerStatuses is the set of statuses that can trigger escalation.
// Only non-terminal, non-resolved statuses are valid triggers.
var validTriggerStatuses = map[string]bool{
	StatusOpen:          true,
	StatusAcknowledged:  true,
	StatusInvestigating: true,
}

// EscalationRule represents a configurable escalation policy.
// Rules are data (DB rows), not code — evaluated by the escalation engine.
type EscalationRule struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenant_id"`
	Name            string                 `json:"name"`
	SeverityMatch   string                 `json:"severity_match"`
	TriggerAfterHrs int                    `json:"trigger_after_hrs"`
	TriggerStatus   string                 `json:"trigger_status"`
	Action          string                 `json:"action"`
	ActionConfig    map[string]interface{} `json:"action_config"`
	IsActive        bool                   `json:"is_active"`
	Priority        int                    `json:"priority"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// ValidAction returns true if the given string is a valid escalation action.
func ValidAction(a string) bool {
	return validActions[a]
}

// ValidSeverityMatch returns true if the given string is a valid severity
// match value, including the "*" wildcard.
func ValidSeverityMatch(s string) bool {
	return validSeverityMatches[s]
}

// ValidTriggerStatus returns true if the given string is a valid trigger
// status for an escalation rule.
func ValidTriggerStatus(s string) bool {
	return validTriggerStatuses[s]
}

// MatchesSeverity returns true if this rule matches the given discrepancy
// severity. A wildcard severity_match ("*") matches any severity.
func (r *EscalationRule) MatchesSeverity(discrepancySeverity string) bool {
	if r.SeverityMatch == "*" {
		return true
	}
	return r.SeverityMatch == discrepancySeverity
}

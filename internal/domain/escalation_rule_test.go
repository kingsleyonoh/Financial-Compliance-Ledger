package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------- Action Constants ----------

func TestActionConstants(t *testing.T) {
	assert.Equal(t, "notify", ActionNotify)
	assert.Equal(t, "escalate", ActionEscalate)
	assert.Equal(t, "auto_close", ActionAutoClose)
}

// ---------- ValidAction ----------

func TestValidAction_AllValid(t *testing.T) {
	validActions := []string{ActionNotify, ActionEscalate, ActionAutoClose}
	for _, a := range validActions {
		assert.True(t, ValidAction(a), "%s should be a valid action", a)
	}
}

func TestValidAction_Invalid(t *testing.T) {
	invalidActions := []string{
		"",
		"delete",
		"NOTIFY",
		"Escalate",
		"auto-close",
		"unknown",
	}
	for _, a := range invalidActions {
		assert.False(t, ValidAction(a), "%q should be an invalid action", a)
	}
}

// ---------- ValidSeverityMatch ----------

func TestValidSeverityMatch_AllValid(t *testing.T) {
	validSeverities := []string{
		SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical,
		"*", // wildcard
	}
	for _, s := range validSeverities {
		assert.True(t, ValidSeverityMatch(s), "%s should be a valid severity match", s)
	}
}

func TestValidSeverityMatch_Invalid(t *testing.T) {
	invalidSeverities := []string{
		"",
		"info",
		"LOW",
		"warning",
		"**",
		"all",
	}
	for _, s := range invalidSeverities {
		assert.False(t, ValidSeverityMatch(s), "%q should be an invalid severity match", s)
	}
}

// ---------- ValidTriggerStatus ----------

func TestValidTriggerStatus_AllValid(t *testing.T) {
	validStatuses := []string{StatusOpen, StatusAcknowledged, StatusInvestigating}
	for _, s := range validStatuses {
		assert.True(t, ValidTriggerStatus(s), "%s should be a valid trigger status", s)
	}
}

func TestValidTriggerStatus_Invalid(t *testing.T) {
	invalidStatuses := []string{
		"",
		StatusResolved,
		StatusEscalated,
		StatusAutoClosed,
		"unknown",
		"OPEN",
	}
	for _, s := range invalidStatuses {
		assert.False(t, ValidTriggerStatus(s), "%q should be an invalid trigger status", s)
	}
}

// ---------- MatchesSeverity ----------

func TestMatchesSeverity_WildcardMatchesAll(t *testing.T) {
	rule := EscalationRule{SeverityMatch: "*"}
	severities := []string{SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	for _, s := range severities {
		assert.True(t, rule.MatchesSeverity(s),
			"wildcard rule should match severity %q", s)
	}
}

func TestMatchesSeverity_ExactMatch(t *testing.T) {
	rule := EscalationRule{SeverityMatch: SeverityHigh}
	assert.True(t, rule.MatchesSeverity(SeverityHigh))
}

func TestMatchesSeverity_NoMatch(t *testing.T) {
	rule := EscalationRule{SeverityMatch: SeverityHigh}
	assert.False(t, rule.MatchesSeverity(SeverityLow))
	assert.False(t, rule.MatchesSeverity(SeverityMedium))
	assert.False(t, rule.MatchesSeverity(SeverityCritical))
}

func TestMatchesSeverity_EmptyDiscrepancySeverity(t *testing.T) {
	rule := EscalationRule{SeverityMatch: SeverityHigh}
	assert.False(t, rule.MatchesSeverity(""))
}

func TestMatchesSeverity_WildcardMatchesEmptyString(t *testing.T) {
	rule := EscalationRule{SeverityMatch: "*"}
	assert.True(t, rule.MatchesSeverity(""))
}

// ---------- EscalationRule Struct ----------

func TestEscalationRuleStruct_HasRequiredFields(t *testing.T) {
	config := map[string]interface{}{"channel": "#alerts"}
	rule := EscalationRule{
		ID:              "rule-id",
		TenantID:        "tenant-id",
		Name:            "Critical Alert",
		SeverityMatch:   SeverityCritical,
		TriggerAfterHrs: 24,
		TriggerStatus:   StatusOpen,
		Action:          ActionNotify,
		ActionConfig:    config,
		IsActive:        true,
		Priority:        1,
	}

	assert.Equal(t, "rule-id", rule.ID)
	assert.Equal(t, "tenant-id", rule.TenantID)
	assert.Equal(t, "Critical Alert", rule.Name)
	assert.Equal(t, SeverityCritical, rule.SeverityMatch)
	assert.Equal(t, 24, rule.TriggerAfterHrs)
	assert.Equal(t, StatusOpen, rule.TriggerStatus)
	assert.Equal(t, ActionNotify, rule.Action)
	assert.Equal(t, config, rule.ActionConfig)
	assert.True(t, rule.IsActive)
	assert.Equal(t, 1, rule.Priority)
}

func TestEscalationRuleStruct_DefaultValues(t *testing.T) {
	rule := EscalationRule{}
	assert.Empty(t, rule.ID)
	assert.False(t, rule.IsActive)
	assert.Equal(t, 0, rule.Priority)
	assert.Equal(t, 0, rule.TriggerAfterHrs)
	assert.Nil(t, rule.ActionConfig)
}

package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Status Constants ----------

func TestStatusConstants(t *testing.T) {
	assert.Equal(t, "open", StatusOpen)
	assert.Equal(t, "acknowledged", StatusAcknowledged)
	assert.Equal(t, "investigating", StatusInvestigating)
	assert.Equal(t, "resolved", StatusResolved)
	assert.Equal(t, "escalated", StatusEscalated)
	assert.Equal(t, "auto_closed", StatusAutoClosed)
}

// ---------- Severity Constants ----------

func TestSeverityConstants(t *testing.T) {
	assert.Equal(t, "low", SeverityLow)
	assert.Equal(t, "medium", SeverityMedium)
	assert.Equal(t, "high", SeverityHigh)
	assert.Equal(t, "critical", SeverityCritical)
}

// ---------- DiscrepancyType Constants ----------

func TestDiscrepancyTypeConstants(t *testing.T) {
	assert.Equal(t, "missing", TypeMissing)
	assert.Equal(t, "mismatch", TypeMismatch)
	assert.Equal(t, "duplicate", TypeDuplicate)
	assert.Equal(t, "timing", TypeTiming)
}

// ---------- Valid Transitions ----------

func TestValidTransition_OpenToAcknowledged(t *testing.T) {
	assert.True(t, ValidTransition(StatusOpen, StatusAcknowledged))
}

func TestValidTransition_OpenToAutoClosed(t *testing.T) {
	assert.True(t, ValidTransition(StatusOpen, StatusAutoClosed))
}

func TestValidTransition_AcknowledgedToInvestigating(t *testing.T) {
	assert.True(t, ValidTransition(StatusAcknowledged, StatusInvestigating))
}

func TestValidTransition_InvestigatingToResolved(t *testing.T) {
	assert.True(t, ValidTransition(StatusInvestigating, StatusResolved))
}

func TestValidTransition_InvestigatingToEscalated(t *testing.T) {
	assert.True(t, ValidTransition(StatusInvestigating, StatusEscalated))
}

func TestValidTransition_EscalatedToResolved(t *testing.T) {
	assert.True(t, ValidTransition(StatusEscalated, StatusResolved))
}

// ---------- Invalid Transitions ----------

func TestInvalidTransition_OpenToResolved(t *testing.T) {
	assert.False(t, ValidTransition(StatusOpen, StatusResolved))
}

func TestInvalidTransition_OpenToInvestigating(t *testing.T) {
	assert.False(t, ValidTransition(StatusOpen, StatusInvestigating))
}

func TestInvalidTransition_OpenToEscalated(t *testing.T) {
	assert.False(t, ValidTransition(StatusOpen, StatusEscalated))
}

func TestInvalidTransition_AcknowledgedToResolved(t *testing.T) {
	assert.False(t, ValidTransition(StatusAcknowledged, StatusResolved))
}

func TestInvalidTransition_AcknowledgedToEscalated(t *testing.T) {
	assert.False(t, ValidTransition(StatusAcknowledged, StatusEscalated))
}

func TestInvalidTransition_AcknowledgedToAutoClosed(t *testing.T) {
	assert.False(t, ValidTransition(StatusAcknowledged, StatusAutoClosed))
}

func TestInvalidTransition_AcknowledgedToOpen(t *testing.T) {
	assert.False(t, ValidTransition(StatusAcknowledged, StatusOpen))
}

func TestInvalidTransition_InvestigatingToOpen(t *testing.T) {
	assert.False(t, ValidTransition(StatusInvestigating, StatusOpen))
}

func TestInvalidTransition_InvestigatingToAcknowledged(t *testing.T) {
	assert.False(t, ValidTransition(StatusInvestigating, StatusAcknowledged))
}

func TestInvalidTransition_InvestigatingToAutoClosed(t *testing.T) {
	assert.False(t, ValidTransition(StatusInvestigating, StatusAutoClosed))
}

func TestInvalidTransition_EscalatedToOpen(t *testing.T) {
	assert.False(t, ValidTransition(StatusEscalated, StatusOpen))
}

func TestInvalidTransition_EscalatedToAcknowledged(t *testing.T) {
	assert.False(t, ValidTransition(StatusEscalated, StatusAcknowledged))
}

func TestInvalidTransition_EscalatedToInvestigating(t *testing.T) {
	assert.False(t, ValidTransition(StatusEscalated, StatusInvestigating))
}

func TestInvalidTransition_EscalatedToAutoClosed(t *testing.T) {
	assert.False(t, ValidTransition(StatusEscalated, StatusAutoClosed))
}

func TestInvalidTransition_EscalatedToEscalated(t *testing.T) {
	assert.False(t, ValidTransition(StatusEscalated, StatusEscalated))
}

func TestInvalidTransition_ResolvedToAnything(t *testing.T) {
	statuses := []string{StatusOpen, StatusAcknowledged, StatusInvestigating, StatusEscalated, StatusAutoClosed, StatusResolved}
	for _, s := range statuses {
		assert.False(t, ValidTransition(StatusResolved, s), "resolved -> %s should be invalid", s)
	}
}

func TestInvalidTransition_AutoClosedToAnything(t *testing.T) {
	statuses := []string{StatusOpen, StatusAcknowledged, StatusInvestigating, StatusEscalated, StatusResolved, StatusAutoClosed}
	for _, s := range statuses {
		assert.False(t, ValidTransition(StatusAutoClosed, s), "auto_closed -> %s should be invalid", s)
	}
}

func TestInvalidTransition_SameStatus(t *testing.T) {
	statuses := []string{StatusOpen, StatusAcknowledged, StatusInvestigating, StatusResolved, StatusEscalated, StatusAutoClosed}
	for _, s := range statuses {
		assert.False(t, ValidTransition(s, s), "%s -> %s (self-transition) should be invalid", s, s)
	}
}

func TestInvalidTransition_EmptyStrings(t *testing.T) {
	assert.False(t, ValidTransition("", StatusOpen))
	assert.False(t, ValidTransition(StatusOpen, ""))
	assert.False(t, ValidTransition("", ""))
}

func TestInvalidTransition_UnknownStatus(t *testing.T) {
	assert.False(t, ValidTransition("unknown", StatusOpen))
	assert.False(t, ValidTransition(StatusOpen, "unknown"))
}

// ---------- TransitionTo ----------

func TestTransitionTo_ValidTransition(t *testing.T) {
	d := &Discrepancy{Status: StatusOpen}
	err := d.TransitionTo(StatusAcknowledged)
	require.NoError(t, err)
	assert.Equal(t, StatusAcknowledged, d.Status)
}

func TestTransitionTo_InvalidTransition(t *testing.T) {
	d := &Discrepancy{Status: StatusOpen}
	err := d.TransitionTo(StatusResolved)
	require.Error(t, err)
	assert.Equal(t, StatusOpen, d.Status, "status should not change on invalid transition")
	assert.Contains(t, err.Error(), "invalid transition")
}

func TestTransitionTo_FullLifecycle(t *testing.T) {
	d := &Discrepancy{Status: StatusOpen}

	// open -> acknowledged
	require.NoError(t, d.TransitionTo(StatusAcknowledged))
	assert.Equal(t, StatusAcknowledged, d.Status)

	// acknowledged -> investigating
	require.NoError(t, d.TransitionTo(StatusInvestigating))
	assert.Equal(t, StatusInvestigating, d.Status)

	// investigating -> escalated
	require.NoError(t, d.TransitionTo(StatusEscalated))
	assert.Equal(t, StatusEscalated, d.Status)

	// escalated -> resolved
	require.NoError(t, d.TransitionTo(StatusResolved))
	assert.Equal(t, StatusResolved, d.Status)
}

func TestTransitionTo_DirectResolve(t *testing.T) {
	d := &Discrepancy{Status: StatusOpen}

	require.NoError(t, d.TransitionTo(StatusAcknowledged))
	require.NoError(t, d.TransitionTo(StatusInvestigating))
	require.NoError(t, d.TransitionTo(StatusResolved))
	assert.Equal(t, StatusResolved, d.Status)
}

func TestTransitionTo_AutoClose(t *testing.T) {
	d := &Discrepancy{Status: StatusOpen}
	require.NoError(t, d.TransitionTo(StatusAutoClosed))
	assert.Equal(t, StatusAutoClosed, d.Status)
}

// ---------- Discrepancy Struct ----------

func TestDiscrepancyStruct_HasRequiredFields(t *testing.T) {
	d := Discrepancy{
		ID:              "test-id",
		TenantID:        "tenant-id",
		ExternalID:      "ext-id",
		SourceSystem:    "test-system",
		DiscrepancyType: TypeMissing,
		Severity:        SeverityHigh,
		Status:          StatusOpen,
		Title:           "Test Discrepancy",
	}

	assert.Equal(t, "test-id", d.ID)
	assert.Equal(t, "tenant-id", d.TenantID)
	assert.Equal(t, "ext-id", d.ExternalID)
	assert.Equal(t, "test-system", d.SourceSystem)
	assert.Equal(t, TypeMissing, d.DiscrepancyType)
	assert.Equal(t, SeverityHigh, d.Severity)
	assert.Equal(t, StatusOpen, d.Status)
	assert.Equal(t, "Test Discrepancy", d.Title)
}

func TestDiscrepancyStruct_OptionalFields(t *testing.T) {
	d := Discrepancy{}

	// Optional pointer fields should be nil by default.
	assert.Nil(t, d.AmountExpected)
	assert.Nil(t, d.AmountActual)
	assert.Nil(t, d.ResolvedAt)
	assert.Nil(t, d.Description)
}

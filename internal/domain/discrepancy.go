// Package domain contains the core business entities and logic for the
// Financial Compliance Ledger. This package has no external dependencies —
// only pure Go structs, constants, and domain logic.
package domain

import (
	"fmt"
	"time"
)

// Status constants represent the lifecycle states of a discrepancy.
const (
	StatusOpen          = "open"
	StatusAcknowledged  = "acknowledged"
	StatusInvestigating = "investigating"
	StatusResolved      = "resolved"
	StatusEscalated     = "escalated"
	StatusAutoClosed    = "auto_closed"
)

// Severity constants represent the severity levels of a discrepancy.
const (
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// DiscrepancyType constants represent the types of financial discrepancies.
const (
	TypeMissing   = "missing"
	TypeMismatch  = "mismatch"
	TypeDuplicate = "duplicate"
	TypeTiming    = "timing"
)

// validTransitions defines the allowed state machine transitions.
// Key = current status, Value = set of allowed next statuses.
var validTransitions = map[string]map[string]bool{
	StatusOpen: {
		StatusAcknowledged: true,
		StatusAutoClosed:   true,
	},
	StatusAcknowledged: {
		StatusInvestigating: true,
	},
	StatusInvestigating: {
		StatusResolved:  true,
		StatusEscalated: true,
	},
	StatusEscalated: {
		StatusResolved: true,
	},
	// resolved and auto_closed are terminal states — no transitions out.
}

// Discrepancy represents a tracked financial discrepancy.
type Discrepancy struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenant_id"`
	ExternalID      string                 `json:"external_id"`
	SourceSystem    string                 `json:"source_system"`
	DiscrepancyType string                 `json:"discrepancy_type"`
	Severity        string                 `json:"severity"`
	Status          string                 `json:"status"`
	Title           string                 `json:"title"`
	Description     *string                `json:"description,omitempty"`
	AmountExpected  *float64               `json:"amount_expected,omitempty"`
	AmountActual    *float64               `json:"amount_actual,omitempty"`
	Currency        string                 `json:"currency"`
	Metadata        map[string]interface{} `json:"metadata"`
	FirstDetectedAt time.Time              `json:"first_detected_at"`
	ResolvedAt      *time.Time             `json:"resolved_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// ValidTransition checks whether a state machine transition is allowed.
func ValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

// TransitionTo attempts to transition the discrepancy to a new status.
// Returns an error if the transition is not allowed by the state machine.
func (d *Discrepancy) TransitionTo(newStatus string) error {
	if !ValidTransition(d.Status, newStatus) {
		return fmt.Errorf(
			"invalid transition from %q to %q",
			d.Status, newStatus,
		)
	}
	d.Status = newStatus
	return nil
}

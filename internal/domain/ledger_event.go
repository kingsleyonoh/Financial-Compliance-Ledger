package domain

import (
	"time"

	"github.com/google/uuid"
)

// Event type constants match the CHECK constraint values in
// the ledger_events table (migration 003).
const (
	EventReceived             = "discrepancy.received"
	EventAcknowledged         = "discrepancy.acknowledged"
	EventInvestigationStarted = "discrepancy.investigation_started"
	EventNoteAdded            = "discrepancy.note_added"
	EventEscalated            = "discrepancy.escalated"
	EventResolved             = "discrepancy.resolved"
	EventAutoClosed           = "discrepancy.auto_closed"
)

// Actor type constants match the CHECK constraint values in
// the ledger_events table (migration 003).
const (
	ActorSystem     = "system"
	ActorUser       = "user"
	ActorEscalation = "escalation"
)

// validEventTypes is the set of all allowed event types.
var validEventTypes = map[string]bool{
	EventReceived:             true,
	EventAcknowledged:         true,
	EventInvestigationStarted: true,
	EventNoteAdded:            true,
	EventEscalated:            true,
	EventResolved:             true,
	EventAutoClosed:           true,
}

// validActorTypes is the set of all allowed actor types.
var validActorTypes = map[string]bool{
	ActorSystem:     true,
	ActorUser:       true,
	ActorEscalation: true,
}

// LedgerEvent represents an immutable event in the audit trail.
// These are append-only: once created, they are never updated or deleted.
type LedgerEvent struct {
	ID             string                 `json:"id"`
	TenantID       string                 `json:"tenant_id"`
	DiscrepancyID  string                 `json:"discrepancy_id"`
	EventType      string                 `json:"event_type"`
	Actor          string                 `json:"actor"`
	ActorType      string                 `json:"actor_type"`
	Payload        map[string]interface{} `json:"payload"`
	SequenceNum    int64                  `json:"sequence_num"`
	CreatedAt      time.Time              `json:"created_at"`
}

// ValidEventType returns true if the given string is a valid event type.
func ValidEventType(t string) bool {
	return validEventTypes[t]
}

// ValidActorType returns true if the given string is a valid actor type.
func ValidActorType(t string) bool {
	return validActorTypes[t]
}

// NewLedgerEvent creates a new LedgerEvent with a generated UUID and
// the current timestamp. The caller is responsible for validating
// event type and actor type before calling this function.
func NewLedgerEvent(
	tenantID, discrepancyID, eventType, actor, actorType string,
	payload map[string]interface{},
) *LedgerEvent {
	return &LedgerEvent{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		DiscrepancyID: discrepancyID,
		EventType:     eventType,
		Actor:         actor,
		ActorType:     actorType,
		Payload:       payload,
		CreatedAt:     time.Now(),
	}
}

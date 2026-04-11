package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// ingestEnvelope is the JSON structure of an inbound NATS message.
type ingestEnvelope struct {
	EventType string         `json:"event_type"`
	Source    string         `json:"source"`
	TenantID string         `json:"tenant_id"`
	Timestamp string         `json:"timestamp"`
	Payload  ingestPayload  `json:"payload"`
}

// validate checks that all required envelope fields are present.
func (e *ingestEnvelope) validate() error {
	if e.EventType == "" {
		return fmt.Errorf("missing required field: event_type")
	}
	if e.Source == "" {
		return fmt.Errorf("missing required field: source")
	}
	if e.TenantID == "" {
		return fmt.Errorf("missing required field: tenant_id")
	}
	if e.Timestamp == "" {
		return fmt.Errorf("missing required field: timestamp")
	}
	return nil
}

// ingestPayload holds the discrepancy details from the NATS message.
type ingestPayload struct {
	ExternalID      string                 `json:"external_id"`
	SourceSystem    string                 `json:"source_system"`
	DiscrepancyType string                 `json:"discrepancy_type"`
	Severity        string                 `json:"severity"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	AmountExpected  *float64               `json:"amount_expected"`
	AmountActual    *float64               `json:"amount_actual"`
	Currency        string                 `json:"currency"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// validate checks that all required payload fields are present.
func (p *ingestPayload) validate() error {
	if p.ExternalID == "" {
		return fmt.Errorf("missing required payload field: external_id")
	}
	if p.Severity == "" {
		return fmt.Errorf("missing required payload field: severity")
	}
	if p.Title == "" {
		return fmt.Errorf("missing required payload field: title")
	}
	if p.DiscrepancyType == "" {
		return fmt.Errorf("missing required payload field: discrepancy_type")
	}
	return nil
}

// shouldDeadLetter checks the delivery count and terminates the message
// if it has exceeded the maximum retry threshold.
func (ing *Ingestion) shouldDeadLetter(msg *nats.Msg) bool {
	meta, err := msg.Metadata()
	if err != nil {
		// Can't determine delivery count — let it proceed
		return false
	}

	if meta.NumDelivered > uint64(maxDeliveryAttempts) {
		ing.logger.Error().
			Uint64("num_delivered", meta.NumDelivered).
			Msg("max delivery attempts exceeded, dead-lettering message")
		_ = msg.Term()
		return true
	}
	return false
}

// validateTenant checks that the tenant_id exists and is active.
// Returns nil on success. On failure, NAKs the message and returns
// an error.
func (ing *Ingestion) validateTenant(msg *nats.Msg, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tid, err := uuid.Parse(tenantID)
	if err != nil {
		ing.logger.Warn().
			Str("tenant_id", tenantID).
			Msg("invalid tenant_id format, rejecting")
		_ = msg.Nak()
		return fmt.Errorf("ingestion.validateTenant: invalid UUID: %w", err)
	}

	var exists bool
	err = ing.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM tenants WHERE id = $1 AND is_active = true)`,
		tid,
	).Scan(&exists)
	if err != nil {
		ing.logger.Error().Err(err).
			Str("tenant_id", tenantID).
			Msg("failed to query tenant, rejecting")
		_ = msg.Nak()
		return fmt.Errorf("ingestion.validateTenant: query: %w", err)
	}

	if !exists {
		ing.logger.Warn().
			Str("tenant_id", tenantID).
			Msg("unknown or inactive tenant, rejecting")
		_ = msg.Nak()
		return fmt.Errorf("ingestion.validateTenant: tenant not found or inactive")
	}

	return nil
}

// isDuplicate checks whether a discrepancy with the given external_id
// already exists for the tenant. If it does, ACKs the message
// (idempotent skip) and returns true.
func (ing *Ingestion) isDuplicate(
	msg *nats.Msg, tenantID, externalID string,
) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tid, _ := uuid.Parse(tenantID) // already validated
	_, err := ing.discrepancyStore.GetByExternalID(ctx, tid, externalID)
	if err == nil {
		// Record exists — this is a duplicate, ACK and skip
		ing.logger.Info().
			Str("tenant_id", tenantID).
			Str("external_id", externalID).
			Msg("duplicate external_id, skipping (idempotent ACK)")
		_ = msg.Ack()
		return true
	}
	// pgx.ErrNoRows means not found — not a duplicate
	return false
}

// persistDiscrepancy creates the discrepancy record and appends a
// "discrepancy.received" ledger event, then ACKs the message.
func (ing *Ingestion) persistDiscrepancy(
	msg *nats.Msg, env *ingestEnvelope,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tid, _ := uuid.Parse(env.TenantID) // already validated

	var desc *string
	if env.Payload.Description != "" {
		desc = &env.Payload.Description
	}

	disc := &domain.Discrepancy{
		ExternalID:      env.Payload.ExternalID,
		SourceSystem:    env.Payload.SourceSystem,
		DiscrepancyType: env.Payload.DiscrepancyType,
		Severity:        env.Payload.Severity,
		Status:          domain.StatusOpen,
		Title:           env.Payload.Title,
		Description:     desc,
		AmountExpected:  env.Payload.AmountExpected,
		AmountActual:    env.Payload.AmountActual,
		Currency:        env.Payload.Currency,
		Metadata:        env.Payload.Metadata,
	}

	created, err := ing.discrepancyStore.Create(ctx, tid, disc)
	if err != nil {
		ing.logger.Error().Err(err).
			Str("tenant_id", env.TenantID).
			Str("external_id", env.Payload.ExternalID).
			Msg("failed to create discrepancy, rejecting")
		_ = msg.Nak()
		return
	}

	// Append ledger event
	ledgerEvent := domain.NewLedgerEvent(
		env.TenantID,
		created.ID,
		domain.EventReceived,
		"nats-ingestion",
		domain.ActorSystem,
		map[string]interface{}{
			"source":      env.Source,
			"external_id": env.Payload.ExternalID,
			"severity":    env.Payload.Severity,
		},
	)

	_, err = ing.eventStore.Append(ctx, tid, ledgerEvent)
	if err != nil {
		ing.logger.Error().Err(err).
			Str("tenant_id", env.TenantID).
			Str("discrepancy_id", created.ID).
			Msg("failed to append ledger event")
		// Discrepancy was created but event failed — still ACK to avoid
		// re-creating the discrepancy (dedup will catch it on retry).
	}

	ing.logger.Info().
		Str("tenant_id", env.TenantID).
		Str("discrepancy_id", created.ID).
		Str("external_id", env.Payload.ExternalID).
		Msg("discrepancy ingested successfully")
	_ = msg.Ack()
}

// Ensure pgx.ErrNoRows is available for duplicate checking.
var _ = pgx.ErrNoRows

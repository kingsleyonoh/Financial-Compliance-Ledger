// Package engine contains the business logic processors for the
// Financial Compliance Ledger: event ingestion, escalation evaluation,
// and RAG feed synchronization.
package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// maxDeliveryAttempts is the threshold after which a message is dead-lettered.
const maxDeliveryAttempts = 3

// Ingestion consumes discrepancy events from NATS JetStream, validates
// them, creates discrepancy records, and appends ledger events.
type Ingestion struct {
	nc               *nats.Conn
	js               nats.JetStreamContext
	sub              *nats.Subscription
	discrepancyStore *store.DiscrepancyStore
	eventStore       *store.EventStore
	pool             *pgxpool.Pool
	logger           zerolog.Logger
	subject          string
	consumerName     string
}

// NewIngestion creates a new Ingestion consumer. It connects to NATS,
// obtains a JetStream context, but does not start consuming until
// Start is called.
func NewIngestion(
	cfg *config.Config,
	discrepancyStore *store.DiscrepancyStore,
	eventStore *store.EventStore,
	pool *pgxpool.Pool,
	logger zerolog.Logger,
) (*Ingestion, error) {
	opts := []nats.Option{
		nats.Name("compliance-ledger-ingestion"),
	}
	if cfg.NATSToken != "" {
		opts = append(opts, nats.Token(cfg.NATSToken))
	}

	nc, err := nats.Connect(cfg.NATSURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("ingestion.New: connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("ingestion.New: get JetStream context: %w", err)
	}

	return &Ingestion{
		nc:               nc,
		js:               js,
		discrepancyStore: discrepancyStore,
		eventStore:       eventStore,
		pool:             pool,
		logger:           logger.With().Str("component", "ingestion").Logger(),
		subject:          cfg.NATSSubject,
		consumerName:     cfg.NATSConsumerName,
	}, nil
}

// Start begins consuming messages from the configured NATS subject.
// It creates a durable pull-based subscription and processes messages
// asynchronously.
func (ing *Ingestion) Start(_ context.Context) error {
	sub, err := ing.js.Subscribe(
		ing.subject,
		ing.handleMessage,
		nats.Durable(ing.consumerName),
		nats.ManualAck(),
		nats.AckWait(nats.DefaultTimeout),
	)
	if err != nil {
		return fmt.Errorf("ingestion.Start: subscribe: %w", err)
	}

	ing.sub = sub
	ing.logger.Info().
		Str("subject", ing.subject).
		Str("consumer", ing.consumerName).
		Msg("ingestion started")
	return nil
}

// Stop drains the NATS connection and closes the subscription.
func (ing *Ingestion) Stop() error {
	if ing.sub != nil {
		if err := ing.sub.Drain(); err != nil {
			ing.logger.Warn().Err(err).Msg("failed to drain subscription")
		}
	}
	if ing.nc != nil {
		if err := ing.nc.Drain(); err != nil {
			return fmt.Errorf("ingestion.Stop: drain: %w", err)
		}
	}
	ing.logger.Info().Msg("ingestion stopped")
	return nil
}

// handleMessage processes a single NATS message through the ingestion
// pipeline: parse, validate, deduplicate, persist.
func (ing *Ingestion) handleMessage(msg *nats.Msg) {
	// Check delivery count for dead-letter handling
	if ing.shouldDeadLetter(msg) {
		return
	}

	// Parse envelope
	var env ingestEnvelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		ing.logger.Warn().Err(err).Msg("malformed JSON, rejecting message")
		_ = msg.Nak()
		return
	}

	// Validate envelope required fields
	if err := env.validate(); err != nil {
		ing.logger.Warn().Err(err).Msg("invalid envelope, rejecting message")
		_ = msg.Nak()
		return
	}

	// Validate payload required fields
	if err := env.Payload.validate(); err != nil {
		ing.logger.Warn().
			Err(err).
			Str("tenant_id", env.TenantID).
			Msg("invalid payload, rejecting message")
		_ = msg.Nak()
		return
	}

	// Validate tenant exists and is active
	if err := ing.validateTenant(msg, env.TenantID); err != nil {
		return // validateTenant already handled NAK
	}

	// Check for duplicate external_id (idempotent skip)
	if ing.isDuplicate(msg, env.TenantID, env.Payload.ExternalID) {
		return // isDuplicate already ACK'd
	}

	// Create discrepancy and append ledger event
	ing.persistDiscrepancy(msg, &env)
}

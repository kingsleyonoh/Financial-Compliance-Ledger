package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// EventStore provides append-only database operations for ledger events.
// Events are immutable — once created, they are never updated or deleted.
type EventStore struct {
	pool *pgxpool.Pool
}

// NewEventStore creates a new EventStore.
func NewEventStore(pool *pgxpool.Pool) *EventStore {
	return &EventStore{pool: pool}
}

// Append inserts a new immutable event and returns it with generated fields
// (id, sequence_num, created_at). This is the only write operation — no
// updates or deletes are permitted on ledger events.
func (s *EventStore) Append(
	ctx context.Context, tenantID uuid.UUID, event *domain.LedgerEvent,
) (*domain.LedgerEvent, error) {
	return AppendWith(ctx, s.pool, tenantID, event)
}

// AppendWith inserts a new event using the provided DBTX (pool or tx).
func AppendWith(
	ctx context.Context, db DBTX, tenantID uuid.UUID,
	event *domain.LedgerEvent,
) (*domain.LedgerEvent, error) {
	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return nil, fmt.Errorf("event_store.Append: marshal payload: %w", err)
	}

	var e domain.LedgerEvent
	var payloadOut []byte
	err = db.QueryRow(ctx, `
		INSERT INTO ledger_events
			(tenant_id, discrepancy_id, event_type, actor, actor_type, payload)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, discrepancy_id, event_type, actor,
			actor_type, payload, sequence_num, created_at
	`, tenantID, event.DiscrepancyID, event.EventType,
		event.Actor, event.ActorType, payloadBytes,
	).Scan(
		&e.ID, &e.TenantID, &e.DiscrepancyID, &e.EventType,
		&e.Actor, &e.ActorType, &payloadOut, &e.SequenceNum, &e.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("event_store.Append: %w", err)
	}
	if payloadOut != nil {
		_ = json.Unmarshal(payloadOut, &e.Payload)
	}
	return &e, nil
}

// ListByDiscrepancy returns all events for a discrepancy, ordered by
// sequence_num ASC (chronological order).
func (s *EventStore) ListByDiscrepancy(
	ctx context.Context, tenantID uuid.UUID, discrepancyID uuid.UUID,
) ([]*domain.LedgerEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, discrepancy_id, event_type, actor,
			actor_type, payload, sequence_num, created_at
		FROM ledger_events
		WHERE tenant_id = $1 AND discrepancy_id = $2
		ORDER BY sequence_num ASC
	`, tenantID, discrepancyID)
	if err != nil {
		return nil, fmt.Errorf("event_store.ListByDiscrepancy: %w", err)
	}
	defer rows.Close()

	var events []*domain.LedgerEvent
	for rows.Next() {
		var e domain.LedgerEvent
		var payloadBytes []byte
		err := rows.Scan(
			&e.ID, &e.TenantID, &e.DiscrepancyID, &e.EventType,
			&e.Actor, &e.ActorType, &payloadBytes, &e.SequenceNum,
			&e.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("event_store.ListByDiscrepancy: scan: %w", err)
		}
		if payloadBytes != nil {
			_ = json.Unmarshal(payloadBytes, &e.Payload)
		}
		events = append(events, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event_store.ListByDiscrepancy: rows: %w", err)
	}
	return events, nil
}

// CountByType returns a map of event_type -> count for the given tenant.
func (s *EventStore) CountByType(
	ctx context.Context, tenantID uuid.UUID,
) (map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT event_type, COUNT(*)
		FROM ledger_events
		WHERE tenant_id = $1
		GROUP BY event_type
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("event_store.CountByType: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("event_store.CountByType: scan: %w", err)
		}
		counts[eventType] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event_store.CountByType: rows: %w", err)
	}
	return counts, nil
}

// GetLatestSequence returns the highest sequence_num for events belonging
// to the given discrepancy. Returns 0 if no events exist yet. Used for
// optimistic locking on workflow actions.
func (s *EventStore) GetLatestSequence(
	ctx context.Context, tenantID uuid.UUID, discrepancyID uuid.UUID,
) (int64, error) {
	return GetLatestSequenceWith(ctx, s.pool, tenantID, discrepancyID)
}

// GetLatestSequenceWith returns the highest sequence_num using the provided
// DBTX (pool or tx). Returns 0 if no events exist yet.
func GetLatestSequenceWith(
	ctx context.Context, db DBTX, tenantID uuid.UUID,
	discrepancyID uuid.UUID,
) (int64, error) {
	var seq *int64
	err := db.QueryRow(ctx, `
		SELECT MAX(sequence_num)
		FROM ledger_events
		WHERE tenant_id = $1 AND discrepancy_id = $2
	`, tenantID, discrepancyID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("event_store.GetLatestSequence: %w", err)
	}
	if seq == nil {
		return 0, nil
	}
	return *seq, nil
}

// ExistsForDiscrepancy checks if an event of the given type already exists
// for the specified discrepancy. Used by the escalation engine for dedup.
func (s *EventStore) ExistsForDiscrepancy(
	ctx context.Context, tenantID uuid.UUID,
	discrepancyID uuid.UUID, eventType string,
) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM ledger_events
			WHERE tenant_id = $1
			  AND discrepancy_id = $2
			  AND event_type = $3
		)
	`, tenantID, discrepancyID, eventType).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("event_store.ExistsForDiscrepancy: %w", err)
	}
	return exists, nil
}

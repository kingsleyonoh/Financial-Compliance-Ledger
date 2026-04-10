package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// ---------- Append + ListByDiscrepancy ----------

func TestEventStore_AppendAndListByDiscrepancy(t *testing.T) {
	pool := newTestPool(t)
	es := store.NewEventStore(pool)
	tenantID := seedTenant(t, pool, "test-event-append")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-evt-001", "high", "open")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Append two events
	e1 := &domain.LedgerEvent{
		TenantID:      tenantID.String(),
		DiscrepancyID: discID.String(),
		EventType:     domain.EventReceived,
		Actor:         "system",
		ActorType:     domain.ActorSystem,
		Payload:       map[string]interface{}{"source": "test"},
	}
	created1, err := es.Append(ctx, tenantID, e1)
	require.NoError(t, err)
	require.NotNil(t, created1)
	assert.NotEmpty(t, created1.ID)
	assert.Greater(t, created1.SequenceNum, int64(0))

	e2 := &domain.LedgerEvent{
		TenantID:      tenantID.String(),
		DiscrepancyID: discID.String(),
		EventType:     domain.EventAcknowledged,
		Actor:         "user@test.com",
		ActorType:     domain.ActorUser,
		Payload:       map[string]interface{}{"note": "acknowledged"},
	}
	created2, err := es.Append(ctx, tenantID, e2)
	require.NoError(t, err)
	assert.Greater(t, created2.SequenceNum, created1.SequenceNum)

	// List by discrepancy — should be ordered by sequence_num ASC
	events, err := es.ListByDiscrepancy(ctx, tenantID, discID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, domain.EventReceived, events[0].EventType)
	assert.Equal(t, domain.EventAcknowledged, events[1].EventType)
	assert.Less(t, events[0].SequenceNum, events[1].SequenceNum)
}

// ---------- CountByType ----------

func TestEventStore_CountByType(t *testing.T) {
	pool := newTestPool(t)
	es := store.NewEventStore(pool)
	tenantID := seedTenant(t, pool, "test-event-count")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-evt-cnt-001", "medium", "open")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Append events of different types
	types := []string{
		domain.EventReceived,
		domain.EventReceived,
		domain.EventAcknowledged,
	}
	for _, et := range types {
		e := &domain.LedgerEvent{
			TenantID:      tenantID.String(),
			DiscrepancyID: discID.String(),
			EventType:     et,
			Actor:         "system",
			ActorType:     domain.ActorSystem,
			Payload:       map[string]interface{}{},
		}
		_, err := es.Append(ctx, tenantID, e)
		require.NoError(t, err)
	}

	counts, err := es.CountByType(ctx, tenantID)
	require.NoError(t, err)
	assert.Equal(t, 2, counts[domain.EventReceived])
	assert.Equal(t, 1, counts[domain.EventAcknowledged])
}

// ---------- ExistsForDiscrepancy ----------

func TestEventStore_ExistsForDiscrepancy(t *testing.T) {
	pool := newTestPool(t)
	es := store.NewEventStore(pool)
	tenantID := seedTenant(t, pool, "test-event-exists")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-evt-ex-001", "low", "open")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Before appending — should not exist
	exists, err := es.ExistsForDiscrepancy(ctx, tenantID, discID, domain.EventEscalated)
	require.NoError(t, err)
	assert.False(t, exists)

	// Append an escalated event
	e := &domain.LedgerEvent{
		TenantID:      tenantID.String(),
		DiscrepancyID: discID.String(),
		EventType:     domain.EventEscalated,
		Actor:         "escalation-engine",
		ActorType:     domain.ActorEscalation,
		Payload:       map[string]interface{}{},
	}
	_, err = es.Append(ctx, tenantID, e)
	require.NoError(t, err)

	// Now it should exist
	exists, err = es.ExistsForDiscrepancy(ctx, tenantID, discID, domain.EventEscalated)
	require.NoError(t, err)
	assert.True(t, exists)

	// Different event type should not exist
	exists, err = es.ExistsForDiscrepancy(ctx, tenantID, discID, domain.EventResolved)
	require.NoError(t, err)
	assert.False(t, exists)
}

// ---------- Tenant Isolation ----------

func TestEventStore_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	es := store.NewEventStore(pool)

	tenantA := seedTenant(t, pool, "tenant-A-evt")
	tenantB := seedTenant(t, pool, "tenant-B-evt")
	discA := seedDiscrepancy(t, pool, tenantA, "ext-evt-iso-a", "high", "open")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantA, tenantB) })

	ctx := context.Background()

	// Append event for tenant A
	e := &domain.LedgerEvent{
		TenantID:      tenantA.String(),
		DiscrepancyID: discA.String(),
		EventType:     domain.EventReceived,
		Actor:         "system",
		ActorType:     domain.ActorSystem,
		Payload:       map[string]interface{}{},
	}
	_, err := es.Append(ctx, tenantA, e)
	require.NoError(t, err)

	// Tenant B should see no events for that discrepancy
	events, err := es.ListByDiscrepancy(ctx, tenantB, discA)
	require.NoError(t, err)
	assert.Empty(t, events)

	// Tenant B's count should be empty
	counts, err := es.CountByType(ctx, tenantB)
	require.NoError(t, err)
	assert.Empty(t, counts)
}

// ---------- Append sets CreatedAt ----------

func TestEventStore_Append_SetsCreatedAt(t *testing.T) {
	pool := newTestPool(t)
	es := store.NewEventStore(pool)
	tenantID := seedTenant(t, pool, "test-event-ts")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-evt-ts-001", "low", "open")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	e := &domain.LedgerEvent{
		TenantID:      tenantID.String(),
		DiscrepancyID: discID.String(),
		EventType:     domain.EventReceived,
		Actor:         "system",
		ActorType:     domain.ActorSystem,
		Payload:       map[string]interface{}{},
	}
	created, err := es.Append(ctx, tenantID, e)
	require.NoError(t, err)
	assert.False(t, created.CreatedAt.IsZero())
}

package handlers_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// seedHandlerTenant inserts a test tenant and returns its UUID.
func seedHandlerTenant(t *testing.T, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	apiKey := fmt.Sprintf("test-api-key-%s", id.String()[:8])
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key, is_active)
		VALUES ($1, $2, $3, true)
	`, id, name, apiKey)
	require.NoError(t, err, "failed to seed tenant")
	return id
}

// seedHandlerDiscrepancy inserts a test discrepancy and returns its UUID.
func seedHandlerDiscrepancy(
	t *testing.T, pool *pgxpool.Pool,
	tenantID uuid.UUID, externalID, severity, status string,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO discrepancies
			(id, tenant_id, external_id, source_system, discrepancy_type,
			 severity, status, title, currency, metadata, first_detected_at)
		VALUES ($1, $2, $3, 'test-system', 'mismatch', $4, $5,
				'Test Discrepancy', 'USD', '{}', NOW())
	`, id, tenantID, externalID, severity, status)
	require.NoError(t, err, "failed to seed discrepancy")
	return id
}

// seedHandlerEvent inserts a test ledger event for a discrepancy.
func seedHandlerEvent(
	t *testing.T, pool *pgxpool.Pool,
	tenantID, discrepancyID uuid.UUID, eventType string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO ledger_events
			(tenant_id, discrepancy_id, event_type, actor, actor_type, payload)
		VALUES ($1, $2, $3, 'test-actor', 'system', '{}')
	`, tenantID, discrepancyID, eventType)
	require.NoError(t, err, "failed to seed event")
}

// cleanupHandlerTenantData deletes all data for the given tenant IDs in
// order that respects foreign key constraints.
func cleanupHandlerTenantData(t *testing.T, pool *pgxpool.Pool, tenantIDs ...uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, tid := range tenantIDs {
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_events WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM escalation_rules WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM reports WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM discrepancies WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
	}
}

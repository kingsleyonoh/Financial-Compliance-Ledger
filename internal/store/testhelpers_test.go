package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// testPoolDSN returns the pgx-compatible PostgreSQL connection string.
func testPoolDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fcl:localdev@localhost:5441/compliance_ledger?sslmode=disable"
	}
	return dsn
}

// newTestPool creates a pgxpool connection pool for testing.
// It ensures migrations are up to date before returning.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testPoolDSN()

	// Run migrations first
	err := store.RunMigrations(dsn, "file://../../migrations")
	require.NoError(t, err, "failed to run migrations")

	pool, err := store.NewPostgresPool(dsn)
	require.NoError(t, err, "failed to create test pool")

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// seedTenant inserts a test tenant and returns its UUID.
func seedTenant(t *testing.T, pool *pgxpool.Pool, name string) uuid.UUID {
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

// seedDiscrepancy inserts a test discrepancy and returns its UUID.
func seedDiscrepancy(
	t *testing.T,
	pool *pgxpool.Pool,
	tenantID uuid.UUID,
	externalID, severity, status string,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metadata, _ := json.Marshal(map[string]interface{}{})
	_, err := pool.Exec(ctx, `
		INSERT INTO discrepancies
			(id, tenant_id, external_id, source_system, discrepancy_type,
			 severity, status, title, currency, metadata, first_detected_at)
		VALUES ($1, $2, $3, 'test-system', 'mismatch', $4, $5,
				'Test Discrepancy', 'USD', $6, NOW())
	`, id, tenantID, externalID, severity, status, metadata)
	require.NoError(t, err, "failed to seed discrepancy")
	return id
}

// cleanupTenantData deletes all data for the given tenant IDs in order
// that respects foreign key constraints.
func cleanupTenantData(t *testing.T, pool *pgxpool.Pool, tenantIDs ...uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, tid := range tenantIDs {
		// Delete in FK-safe order
		_, _ = pool.Exec(ctx, `DELETE FROM notification_log WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_events WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM escalation_rules WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM reports WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM discrepancies WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
	}
}

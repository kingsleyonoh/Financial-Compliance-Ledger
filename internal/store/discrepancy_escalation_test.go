package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// seedDiscrepancyAt inserts a test discrepancy with a specified created_at.
func seedDiscrepancyAt(
	t *testing.T, pool *pgxpool.Pool,
	tenantID uuid.UUID, externalID, severity, status string,
	createdAt time.Time,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metadata, _ := json.Marshal(map[string]interface{}{})
	_, err := pool.Exec(ctx, `
		INSERT INTO discrepancies
			(id, tenant_id, external_id, source_system, discrepancy_type,
			 severity, status, title, currency, metadata,
			 first_detected_at, created_at)
		VALUES ($1, $2, $3, 'test-system', 'mismatch', $4, $5,
				'Test Discrepancy', 'USD', $6, $7, $7)
	`, id, tenantID, externalID, severity, status, metadata, createdAt)
	require.NoError(t, err, "failed to seed discrepancy with timestamp")
	return id
}

func TestDiscrepancyStore_ListForEscalation(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-esc-list")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	// Old discrepancy matching status + severity
	oldID := seedDiscrepancyAt(t, pool, tenantID, "ext-esc-old",
		domain.SeverityHigh, domain.StatusOpen, sixHoursAgo)

	// Recent discrepancy — should NOT match with olderThan = 4 hours ago
	seedDiscrepancyAt(t, pool, tenantID, "ext-esc-recent",
		domain.SeverityHigh, domain.StatusOpen, twoHoursAgo)

	// Different status — should NOT match
	seedDiscrepancyAt(t, pool, tenantID, "ext-esc-ack",
		domain.SeverityHigh, domain.StatusAcknowledged, sixHoursAgo)

	// Different severity — should NOT match
	seedDiscrepancyAt(t, pool, tenantID, "ext-esc-low",
		domain.SeverityLow, domain.StatusOpen, sixHoursAgo)

	olderThan := now.Add(-4 * time.Hour)
	results, err := ds.ListForEscalation(
		ctx, tenantID, domain.StatusOpen, domain.SeverityHigh, olderThan)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, oldID.String(), results[0].ID)
}

func TestDiscrepancyStore_ListForEscalation_WildcardSeverity(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-esc-wildcard")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	seedDiscrepancyAt(t, pool, tenantID, "ext-wc-high",
		domain.SeverityHigh, domain.StatusOpen, sixHoursAgo)
	seedDiscrepancyAt(t, pool, tenantID, "ext-wc-low",
		domain.SeverityLow, domain.StatusOpen, sixHoursAgo)
	seedDiscrepancyAt(t, pool, tenantID, "ext-wc-crit",
		domain.SeverityCritical, domain.StatusOpen, sixHoursAgo)

	olderThan := now.Add(-4 * time.Hour)
	// severity = "*" means match all severities
	results, err := ds.ListForEscalation(
		ctx, tenantID, domain.StatusOpen, "*", olderThan)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestDiscrepancyStore_ListForEscalation_EmptyResult(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-esc-empty")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	olderThan := now.Add(-4 * time.Hour)

	results, err := ds.ListForEscalation(
		ctx, tenantID, domain.StatusOpen, domain.SeverityHigh, olderThan)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestDiscrepancyStore_ListForEscalation_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantA := seedTenant(t, pool, "test-esc-iso-a")
	tenantB := seedTenant(t, pool, "test-esc-iso-b")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantA, tenantB) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)
	olderThan := now.Add(-4 * time.Hour)

	seedDiscrepancyAt(t, pool, tenantA, "ext-iso-a",
		domain.SeverityHigh, domain.StatusOpen, sixHoursAgo)
	seedDiscrepancyAt(t, pool, tenantB, "ext-iso-b",
		domain.SeverityHigh, domain.StatusOpen, sixHoursAgo)

	// Tenant A should only see its own
	results, err := ds.ListForEscalation(
		ctx, tenantA, domain.StatusOpen, domain.SeverityHigh, olderThan)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

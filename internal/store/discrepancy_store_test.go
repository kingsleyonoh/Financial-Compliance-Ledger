package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// ---------- Create + Read ----------

func TestDiscrepancyStore_CreateAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-disc-create")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	expected := float64(100.50)
	actual := float64(90.25)
	desc := "Amount mismatch detected"

	d := &domain.Discrepancy{
		TenantID:        tenantID.String(),
		ExternalID:      "ext-create-001",
		SourceSystem:    "recon-engine",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityHigh,
		Status:          domain.StatusOpen,
		Title:           "Test Create Discrepancy",
		Description:     &desc,
		AmountExpected:  &expected,
		AmountActual:    &actual,
		Currency:        "USD",
		Metadata:        map[string]interface{}{"source": "test"},
	}

	ctx := context.Background()
	created, err := ds.Create(ctx, tenantID, d)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, tenantID.String(), created.TenantID)
	assert.Equal(t, "ext-create-001", created.ExternalID)
	assert.Equal(t, domain.SeverityHigh, created.Severity)
	assert.Equal(t, domain.StatusOpen, created.Status)
	assert.False(t, created.CreatedAt.IsZero())

	// Read back
	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	fetched, err := ds.GetByID(ctx, tenantID, id)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, "ext-create-001", fetched.ExternalID)
	assert.Equal(t, domain.SeverityHigh, fetched.Severity)
	assert.Equal(t, "Test Create Discrepancy", fetched.Title)
	require.NotNil(t, fetched.AmountExpected)
	assert.InDelta(t, 100.50, *fetched.AmountExpected, 0.01)
	require.NotNil(t, fetched.AmountActual)
	assert.InDelta(t, 90.25, *fetched.AmountActual, 0.01)
}

// ---------- GetByExternalID ----------

func TestDiscrepancyStore_GetByExternalID(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-disc-extid")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	d := &domain.Discrepancy{
		TenantID:        tenantID.String(),
		ExternalID:      "ext-lookup-001",
		SourceSystem:    "recon-engine",
		DiscrepancyType: domain.TypeMissing,
		Severity:        domain.SeverityLow,
		Status:          domain.StatusOpen,
		Title:           "Lookup Test",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}

	ctx := context.Background()
	_, err := ds.Create(ctx, tenantID, d)
	require.NoError(t, err)

	fetched, err := ds.GetByExternalID(ctx, tenantID, "ext-lookup-001")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "ext-lookup-001", fetched.ExternalID)
	assert.Equal(t, domain.TypeMissing, fetched.DiscrepancyType)
}

// ---------- Duplicate ExternalID ----------

func TestDiscrepancyStore_DuplicateExternalID(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-disc-dup")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	d := &domain.Discrepancy{
		TenantID:        tenantID.String(),
		ExternalID:      "ext-dup-001",
		SourceSystem:    "recon-engine",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityMedium,
		Status:          domain.StatusOpen,
		Title:           "Dup Test",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}

	ctx := context.Background()
	_, err := ds.Create(ctx, tenantID, d)
	require.NoError(t, err)

	// Same external_id for same tenant should fail
	_, err = ds.Create(ctx, tenantID, d)
	require.Error(t, err, "duplicate external_id should fail")
}

// ---------- List with Filters ----------

func TestDiscrepancyStore_ListWithFilters(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-disc-list")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Insert discrepancies with different severities and statuses
	for i, sev := range []string{domain.SeverityLow, domain.SeverityHigh, domain.SeverityCritical} {
		d := &domain.Discrepancy{
			TenantID:        tenantID.String(),
			ExternalID:      fmt.Sprintf("ext-list-%03d", i),
			SourceSystem:    "recon-engine",
			DiscrepancyType: domain.TypeMismatch,
			Severity:        sev,
			Status:          domain.StatusOpen,
			Title:           fmt.Sprintf("List Test %d", i),
			Currency:        "USD",
			Metadata:        map[string]interface{}{},
		}
		_, err := ds.Create(ctx, tenantID, d)
		require.NoError(t, err)
	}

	// Filter by severity=high
	results, total, err := ds.List(ctx, tenantID, store.ListFilters{
		Severity: "high",
		Limit:    25,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, domain.SeverityHigh, results[0].Severity)

	// Filter by status=open (all 3)
	results, total, err = ds.List(ctx, tenantID, store.ListFilters{
		Status: "open",
		Limit:  25,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, results, 3)
}

// ---------- Cursor-Based Pagination ----------

func TestDiscrepancyStore_CursorPagination(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-disc-cursor")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Insert 5 discrepancies
	var createdIDs []string
	for i := 0; i < 5; i++ {
		d := &domain.Discrepancy{
			TenantID:        tenantID.String(),
			ExternalID:      fmt.Sprintf("ext-page-%03d", i),
			SourceSystem:    "recon-engine",
			DiscrepancyType: domain.TypeMismatch,
			Severity:        domain.SeverityMedium,
			Status:          domain.StatusOpen,
			Title:           fmt.Sprintf("Page Test %d", i),
			Currency:        "USD",
			Metadata:        map[string]interface{}{},
		}
		created, err := ds.Create(ctx, tenantID, d)
		require.NoError(t, err)
		createdIDs = append(createdIDs, created.ID)
	}

	// First page: limit 2
	page1, total, err := ds.List(ctx, tenantID, store.ListFilters{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	require.Len(t, page1, 2)

	// Second page using cursor from last item of page1
	cursor := page1[len(page1)-1].ID
	page2, _, err := ds.List(ctx, tenantID, store.ListFilters{
		Cursor: cursor,
		Limit:  2,
	})
	require.NoError(t, err)
	require.Len(t, page2, 2)

	// Ensure no overlap between pages
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
	assert.NotEqual(t, page1[1].ID, page2[0].ID)
}

// ---------- UpdateStatus ----------

func TestDiscrepancyStore_UpdateStatus(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-disc-update")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	d := &domain.Discrepancy{
		TenantID:        tenantID.String(),
		ExternalID:      "ext-update-001",
		SourceSystem:    "recon-engine",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityHigh,
		Status:          domain.StatusOpen,
		Title:           "Update Test",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	created, err := ds.Create(ctx, tenantID, d)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	// Update status to acknowledged
	err = ds.UpdateStatus(ctx, tenantID, id, domain.StatusAcknowledged, nil)
	require.NoError(t, err)

	fetched, err := ds.GetByID(ctx, tenantID, id)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAcknowledged, fetched.Status)
	assert.Nil(t, fetched.ResolvedAt)

	// Update to resolved with resolved_at
	now := time.Now().UTC()
	err = ds.UpdateStatus(ctx, tenantID, id, domain.StatusResolved, &now)
	require.NoError(t, err)

	fetched, err = ds.GetByID(ctx, tenantID, id)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusResolved, fetched.Status)
	require.NotNil(t, fetched.ResolvedAt)
}

// ---------- Tenant Isolation ----------

func TestDiscrepancyStore_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)

	tenantA := seedTenant(t, pool, "tenant-A-disc")
	tenantB := seedTenant(t, pool, "tenant-B-disc")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantA, tenantB) })

	ctx := context.Background()

	// Create discrepancy for tenant A
	d := &domain.Discrepancy{
		TenantID:        tenantA.String(),
		ExternalID:      "ext-iso-001",
		SourceSystem:    "recon-engine",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityHigh,
		Status:          domain.StatusOpen,
		Title:           "Tenant A Discrepancy",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	created, err := ds.Create(ctx, tenantA, d)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	// Tenant B should NOT see tenant A's discrepancy
	fetched, err := ds.GetByID(ctx, tenantB, id)
	assert.Error(t, err)
	assert.Nil(t, fetched)

	// Tenant B's list should be empty
	results, total, err := ds.List(ctx, tenantB, store.ListFilters{Limit: 25})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}

// ---------- GetByID Not Found ----------

func TestDiscrepancyStore_GetByID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	tenantID := seedTenant(t, pool, "test-disc-notfound")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	nonExistent := uuid.New()
	fetched, err := ds.GetByID(ctx, tenantID, nonExistent)
	assert.Error(t, err)
	assert.Nil(t, fetched)
}

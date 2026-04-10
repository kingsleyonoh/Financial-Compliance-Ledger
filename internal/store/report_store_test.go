package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// ---------- Create + GetByID ----------

func TestReportStore_CreateAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-create")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	report := &domain.Report{
		TenantID:   tenantID.String(),
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Daily Summary 2026-04-10",
		Parameters: map[string]interface{}{"date": "2026-04-10"},
		Status:     domain.ReportStatusPending,
	}

	created, err := rs.Create(ctx, tenantID, report)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, tenantID.String(), created.TenantID)
	assert.Equal(t, domain.ReportTypeDailySummary, created.ReportType)
	assert.Equal(t, "Daily Summary 2026-04-10", created.Title)
	assert.Equal(t, domain.ReportStatusPending, created.Status)
	assert.Nil(t, created.FilePath)
	assert.Nil(t, created.FileSizeBytes)
	assert.False(t, created.CreatedAt.IsZero())
	assert.False(t, created.UpdatedAt.IsZero())

	// Read back by ID
	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	fetched, err := rs.GetByID(ctx, tenantID, id)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, domain.ReportTypeDailySummary, fetched.ReportType)
	assert.Equal(t, "Daily Summary 2026-04-10", fetched.Title)
	assert.Equal(t, domain.ReportStatusPending, fetched.Status)
}

// ---------- GetByID Not Found ----------

func TestReportStore_GetByID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-notfound")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	nonExistent := uuid.New()
	fetched, err := rs.GetByID(ctx, tenantID, nonExistent)
	assert.Error(t, err)
	assert.Nil(t, fetched)
}

// ---------- List (ordered by created_at DESC) ----------

func TestReportStore_List_OrderedByCreatedAtDesc(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-list")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Create three reports with slight delay to ensure different timestamps
	r1 := &domain.Report{
		TenantID:   tenantID.String(),
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Report 1",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	created1, err := rs.Create(ctx, tenantID, r1)
	require.NoError(t, err)

	// Small delay to ensure ordering
	time.Sleep(10 * time.Millisecond)

	r2 := &domain.Report{
		TenantID:   tenantID.String(),
		ReportType: domain.ReportTypeMonthlyAudit,
		Title:      "Report 2",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusCompleted,
	}
	created2, err := rs.Create(ctx, tenantID, r2)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	r3 := &domain.Report{
		TenantID:   tenantID.String(),
		ReportType: domain.ReportTypeCustom,
		Title:      "Report 3",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusGenerating,
	}
	created3, err := rs.Create(ctx, tenantID, r3)
	require.NoError(t, err)

	// List — should be in descending order (newest first)
	reports, err := rs.List(ctx, tenantID)
	require.NoError(t, err)
	require.Len(t, reports, 3)

	// Most recent first
	assert.Equal(t, created3.ID, reports[0].ID)
	assert.Equal(t, created2.ID, reports[1].ID)
	assert.Equal(t, created1.ID, reports[2].ID)
}

// ---------- List Empty ----------

func TestReportStore_List_Empty(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-list-empty")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	reports, err := rs.List(ctx, tenantID)
	require.NoError(t, err)
	assert.Empty(t, reports)
}

// ---------- UpdateStatus to completed with file info ----------

func TestReportStore_UpdateStatus_Completed(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-update")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	report := &domain.Report{
		TenantID:   tenantID.String(),
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Completion Test",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(ctx, tenantID, report)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	filePath := "/data/reports/test-report.pdf"
	fileSize := int64(1024)

	err = rs.UpdateStatus(ctx, tenantID, id, domain.ReportStatusCompleted, &filePath, &fileSize)
	require.NoError(t, err)

	// Verify the update
	fetched, err := rs.GetByID(ctx, tenantID, id)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportStatusCompleted, fetched.Status)
	require.NotNil(t, fetched.FilePath)
	assert.Equal(t, filePath, *fetched.FilePath)
	require.NotNil(t, fetched.FileSizeBytes)
	assert.Equal(t, fileSize, *fetched.FileSizeBytes)
	// Updated_at should be after created_at (or at least equal)
	assert.True(t, !fetched.UpdatedAt.Before(fetched.CreatedAt))
}

// ---------- UpdateStatus without file info (e.g., generating) ----------

func TestReportStore_UpdateStatus_NoFileInfo(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-upd-nofile")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	report := &domain.Report{
		TenantID:   tenantID.String(),
		ReportType: domain.ReportTypeMonthlyAudit,
		Title:      "No File Update",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(ctx, tenantID, report)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	err = rs.UpdateStatus(ctx, tenantID, id, domain.ReportStatusGenerating, nil, nil)
	require.NoError(t, err)

	fetched, err := rs.GetByID(ctx, tenantID, id)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportStatusGenerating, fetched.Status)
	assert.Nil(t, fetched.FilePath)
	assert.Nil(t, fetched.FileSizeBytes)
}

// ---------- UpdateStatus Not Found ----------

func TestReportStore_UpdateStatus_NotFound(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-upd-nf")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	nonExistent := uuid.New()

	err := rs.UpdateStatus(ctx, tenantID, nonExistent, domain.ReportStatusCompleted, nil, nil)
	assert.Error(t, err)
}

// ---------- Tenant Isolation ----------

func TestReportStore_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)

	tenantA := seedTenant(t, pool, "tenant-A-report")
	tenantB := seedTenant(t, pool, "tenant-B-report")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantA, tenantB) })

	ctx := context.Background()

	report := &domain.Report{
		TenantID:   tenantA.String(),
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Tenant A Report",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(ctx, tenantA, report)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	// Tenant B should NOT see tenant A's report
	fetched, err := rs.GetByID(ctx, tenantB, id)
	assert.Error(t, err)
	assert.Nil(t, fetched)

	// Tenant B's list should be empty
	reports, err := rs.List(ctx, tenantB)
	require.NoError(t, err)
	assert.Empty(t, reports)

	// Tenant B should NOT be able to update tenant A's report
	err = rs.UpdateStatus(ctx, tenantB, id, domain.ReportStatusCompleted, nil, nil)
	assert.Error(t, err)
}

// ---------- Create with GeneratedBy ----------

func TestReportStore_Create_WithGeneratedBy(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-genby")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	generatedBy := "admin@example.com"
	report := &domain.Report{
		TenantID:    tenantID.String(),
		ReportType:  domain.ReportTypeDiscrepancyDetail,
		Title:       "Detail Report",
		Parameters:  map[string]interface{}{"discrepancy_id": "abc"},
		Status:      domain.ReportStatusPending,
		GeneratedBy: &generatedBy,
	}

	created, err := rs.Create(ctx, tenantID, report)
	require.NoError(t, err)
	require.NotNil(t, created.GeneratedBy)
	assert.Equal(t, generatedBy, *created.GeneratedBy)
}

// ---------- Parameters stored correctly ----------

func TestReportStore_Create_ParametersPreserved(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "test-report-params")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	params := map[string]interface{}{
		"date_from":   "2026-04-01",
		"date_to":     "2026-04-10",
		"severity":    "critical",
		"include_all": true,
	}
	report := &domain.Report{
		TenantID:   tenantID.String(),
		ReportType: domain.ReportTypeCustom,
		Title:      "Custom Params Report",
		Parameters: params,
		Status:     domain.ReportStatusPending,
	}

	created, err := rs.Create(ctx, tenantID, report)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	fetched, err := rs.GetByID(ctx, tenantID, id)
	require.NoError(t, err)
	assert.Equal(t, "2026-04-01", fetched.Parameters["date_from"])
	assert.Equal(t, "2026-04-10", fetched.Parameters["date_to"])
	assert.Equal(t, "critical", fetched.Parameters["severity"])
	assert.Equal(t, true, fetched.Parameters["include_all"])
}

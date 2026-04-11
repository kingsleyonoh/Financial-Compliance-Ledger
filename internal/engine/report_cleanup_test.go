package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

func TestReportCleaner_Cleanup_DeletesOldFiles(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "cleanup-old-files")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	logger := zerolog.New(zerolog.NewTestWriter(t))

	// Create temp storage directory
	tmpDir, err := os.MkdirTemp("", "report-cleanup-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an old report
	oldReport := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Old Daily Report",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(ctx, tenantID, oldReport)
	require.NoError(t, err)

	// Create a fake PDF file on disk
	reportID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	fakeFile := filepath.Join(tmpDir, reportID.String()+".html")
	require.NoError(t, os.WriteFile(fakeFile, []byte("fake report content"), 0o644))

	// Update report to completed with file path
	fileSize := int64(len("fake report content"))
	err = rs.UpdateStatus(ctx, tenantID, reportID,
		domain.ReportStatusCompleted, &fakeFile, &fileSize)
	require.NoError(t, err)

	// Backdate the report to make it old (> 365 days)
	_, err = pool.Exec(ctx,
		`UPDATE reports SET created_at = $1 WHERE id = $2`,
		time.Now().UTC().Add(-400*24*time.Hour), reportID)
	require.NoError(t, err)

	// Run cleanup
	cleaner := engine.NewReportCleaner(rs, tmpDir, logger)
	err = cleaner.Cleanup(ctx)
	require.NoError(t, err)

	// Verify: file should be deleted
	_, statErr := os.Stat(fakeFile)
	assert.True(t, os.IsNotExist(statErr), "old report file should be deleted")

	// Verify: report status should be "cleaned"
	updated, err := rs.GetByID(ctx, tenantID, reportID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportStatusCleaned, updated.Status)
	assert.Nil(t, updated.FilePath)
}

func TestReportCleaner_Cleanup_SkipsRecentReports(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "cleanup-recent")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tmpDir, err := os.MkdirTemp("", "report-cleanup-recent-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a recent report (< 365 days old)
	recentReport := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Recent Report",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(ctx, tenantID, recentReport)
	require.NoError(t, err)

	reportID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	fakeFile := filepath.Join(tmpDir, reportID.String()+".html")
	require.NoError(t, os.WriteFile(fakeFile, []byte("recent content"), 0o644))

	fileSize := int64(len("recent content"))
	err = rs.UpdateStatus(ctx, tenantID, reportID,
		domain.ReportStatusCompleted, &fakeFile, &fileSize)
	require.NoError(t, err)

	// Run cleanup -- should NOT delete recent report
	cleaner := engine.NewReportCleaner(rs, tmpDir, logger)
	err = cleaner.Cleanup(ctx)
	require.NoError(t, err)

	// Verify: file should still exist
	_, statErr := os.Stat(fakeFile)
	assert.False(t, os.IsNotExist(statErr),
		"recent report file should NOT be deleted")

	// Verify: status should still be completed
	updated, err := rs.GetByID(ctx, tenantID, reportID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportStatusCompleted, updated.Status)
}

func TestReportCleaner_Cleanup_HandlesAlreadyDeletedFiles(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "cleanup-missing-file")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tmpDir, err := os.MkdirTemp("", "report-cleanup-missing-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an old report with a file path that doesn't exist
	oldReport := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Missing File Report",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(ctx, tenantID, oldReport)
	require.NoError(t, err)

	reportID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	missingFile := filepath.Join(tmpDir, "nonexistent.html")
	fileSize := int64(100)
	err = rs.UpdateStatus(ctx, tenantID, reportID,
		domain.ReportStatusCompleted, &missingFile, &fileSize)
	require.NoError(t, err)

	// Backdate
	_, err = pool.Exec(ctx,
		`UPDATE reports SET created_at = $1 WHERE id = $2`,
		time.Now().UTC().Add(-400*24*time.Hour), reportID)
	require.NoError(t, err)

	// Run cleanup -- should succeed even if file doesn't exist
	cleaner := engine.NewReportCleaner(rs, tmpDir, logger)
	err = cleaner.Cleanup(ctx)
	require.NoError(t, err)

	// Status should still be marked as cleaned
	updated, err := rs.GetByID(ctx, tenantID, reportID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportStatusCleaned, updated.Status)
}

func TestReportCleaner_Cleanup_SkipsNonCompletedReports(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewReportStore(pool)
	tenantID := seedTenant(t, pool, "cleanup-failed")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tmpDir, err := os.MkdirTemp("", "report-cleanup-failed-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a failed report (old but not completed)
	failedReport := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Failed Report",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	created, err := rs.Create(ctx, tenantID, failedReport)
	require.NoError(t, err)

	reportID, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	err = rs.UpdateStatus(ctx, tenantID, reportID,
		domain.ReportStatusFailed, nil, nil)
	require.NoError(t, err)

	// Backdate
	_, err = pool.Exec(ctx,
		`UPDATE reports SET created_at = $1 WHERE id = $2`,
		time.Now().UTC().Add(-400*24*time.Hour), reportID)
	require.NoError(t, err)

	// Run cleanup -- should skip failed reports
	cleaner := engine.NewReportCleaner(rs, tmpDir, logger)
	err = cleaner.Cleanup(ctx)
	require.NoError(t, err)

	// Status should remain failed
	updated, err := rs.GetByID(ctx, tenantID, reportID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportStatusFailed, updated.Status)
}

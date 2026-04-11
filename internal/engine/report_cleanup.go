package engine

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// retentionDays is the number of days after which completed reports
// are eligible for cleanup (PDF deletion from disk).
const retentionDays = 365

// ReportCleaner deletes old report PDF files from disk and updates
// their status to "cleaned". Runs as a background goroutine.
type ReportCleaner struct {
	reportStore *store.ReportStore
	storagePath string
	logger      zerolog.Logger
}

// NewReportCleaner creates a new ReportCleaner.
func NewReportCleaner(
	rs *store.ReportStore,
	storagePath string,
	logger zerolog.Logger,
) *ReportCleaner {
	return &ReportCleaner{
		reportStore: rs,
		storagePath: storagePath,
		logger: logger.With().
			Str("component", "report-cleaner").Logger(),
	}
}

// Start runs the cleanup job on a schedule. It checks every hour and
// executes cleanup only when the current UTC hour is 02 (daily at
// 02:00 UTC). Stops when the context is cancelled.
func (rc *ReportCleaner) Start(ctx context.Context) {
	rc.logger.Info().Msg("report cleaner started")

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			rc.logger.Info().Msg("report cleaner stopped")
			return
		case <-ticker.C:
			if time.Now().UTC().Hour() == 2 {
				if err := rc.Cleanup(ctx); err != nil {
					rc.logger.Error().Err(err).
						Msg("report cleanup failed")
				}
			}
		}
	}
}

// Cleanup finds completed reports older than the retention period,
// deletes their PDF files from disk, and marks them as cleaned.
func (rc *ReportCleaner) Cleanup(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(
		-time.Duration(retentionDays) * 24 * time.Hour)

	reports, err := rc.reportStore.ListOlderThan(
		ctx, cutoff, domain.ReportStatusCompleted)
	if err != nil {
		return fmt.Errorf("report_cleanup: list old reports: %w", err)
	}

	if len(reports) == 0 {
		return nil
	}

	var deletedCount int
	var freedBytes int64

	for _, rpt := range reports {
		freed := rc.cleanupReport(ctx, rpt)
		if freed >= 0 {
			deletedCount++
			freedBytes += freed
		}
	}

	rc.logger.Info().
		Int("deleted_files", deletedCount).
		Int64("freed_bytes", freedBytes).
		Int("total_reports", len(reports)).
		Msg("report cleanup completed")

	return nil
}

// cleanupReport deletes a single report's file and marks it as
// cleaned. Returns the freed bytes, or -1 on failure.
func (rc *ReportCleaner) cleanupReport(
	ctx context.Context, rpt *domain.Report,
) int64 {
	var freedBytes int64

	// Delete file from disk if path is set
	if rpt.FilePath != nil && *rpt.FilePath != "" {
		info, err := os.Stat(*rpt.FilePath)
		if err == nil {
			freedBytes = info.Size()
		}

		if err := os.Remove(*rpt.FilePath); err != nil && !os.IsNotExist(err) {
			rc.logger.Warn().Err(err).
				Str("report_id", rpt.ID).
				Str("file_path", *rpt.FilePath).
				Msg("failed to delete report file")
			return -1
		}
	}

	// Mark as cleaned in the database
	reportID, err := uuid.Parse(rpt.ID)
	if err != nil {
		rc.logger.Warn().Err(err).
			Str("report_id", rpt.ID).
			Msg("invalid report ID, skipping")
		return -1
	}

	if err := rc.reportStore.MarkCleaned(ctx, reportID); err != nil {
		rc.logger.Warn().Err(err).
			Str("report_id", rpt.ID).
			Msg("failed to mark report as cleaned")
		return -1
	}

	return freedBytes
}

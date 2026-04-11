package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
)

const metricsSnapshotInterval = 5 * time.Minute

// MetricsCollector periodically queries the database and updates
// Prometheus gauge metrics for discrepancy counts by status and severity.
type MetricsCollector struct {
	pool   *pgxpool.Pool
	logger zerolog.Logger
}

// NewMetricsCollector creates a new MetricsCollector.
func NewMetricsCollector(
	pool *pgxpool.Pool, logger zerolog.Logger,
) *MetricsCollector {
	return &MetricsCollector{
		pool: pool,
		logger: logger.With().
			Str("component", "metrics-collector").Logger(),
	}
}

// Start runs the snapshot loop on a 5-minute interval.
// It stops when the context is cancelled.
func (m *MetricsCollector) Start(ctx context.Context) {
	m.logger.Info().Msg("metrics collector started")

	ticker := time.NewTicker(metricsSnapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info().Msg("metrics collector stopped")
			return
		case <-ticker.C:
			if err := m.Snapshot(ctx); err != nil {
				m.logger.Error().Err(err).
					Msg("metrics snapshot failed")
			}
		}
	}
}

// Snapshot queries current discrepancy counts by status and severity,
// then updates the Prometheus gauges.
func (m *MetricsCollector) Snapshot(ctx context.Context) error {
	// Reset existing gauge values to avoid stale data
	handlers.DiscrepanciesGauge.Reset()

	rows, err := m.pool.Query(ctx, `
		SELECT status, severity, COUNT(*)
		FROM discrepancies
		GROUP BY status, severity
	`)
	if err != nil {
		return fmt.Errorf("metrics.Snapshot: query: %w", err)
	}
	defer rows.Close()

	total := 0
	for rows.Next() {
		var status, severity string
		var count int
		if err := rows.Scan(&status, &severity, &count); err != nil {
			return fmt.Errorf("metrics.Snapshot: scan: %w", err)
		}
		handlers.DiscrepanciesGauge.WithLabelValues(status, severity).
			Set(float64(count))
		total += count
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("metrics.Snapshot: rows: %w", err)
	}

	m.logger.Info().
		Int("total_discrepancies", total).
		Msg("metrics snapshot completed")

	return nil
}

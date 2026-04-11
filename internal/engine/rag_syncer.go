package engine

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

const ragSyncInterval = 1 * time.Hour

// RAGSyncer periodically queries resolved discrepancies and feeds them
// to the RAG Platform via RAGFeeder.
type RAGSyncer struct {
	feeder           *RAGFeeder
	discrepancyStore *store.DiscrepancyStore
	lastSync         time.Time
	logger           zerolog.Logger
}

// NewRAGSyncer creates a new RAGSyncer. On first run it syncs
// discrepancies resolved in the last 24 hours.
func NewRAGSyncer(
	feeder *RAGFeeder,
	ds *store.DiscrepancyStore,
	logger zerolog.Logger,
) *RAGSyncer {
	return &RAGSyncer{
		feeder:           feeder,
		discrepancyStore: ds,
		lastSync:         time.Now().UTC().Add(-24 * time.Hour),
		logger: logger.With().
			Str("component", "rag-syncer").Logger(),
	}
}

// StartSync runs the sync loop on a 1-hour interval.
// It stops when the context is cancelled.
func (s *RAGSyncer) StartSync(ctx context.Context) {
	s.logger.Info().
		Time("last_sync", s.lastSync).
		Msg("RAG syncer started")

	ticker := time.NewTicker(ragSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info().Msg("RAG syncer stopped")
			return
		case <-ticker.C:
			s.runSync(ctx)
		}
	}
}

// runSync performs a single sync pass: queries resolved discrepancies
// since the last sync and feeds each one to the RAG Platform.
func (s *RAGSyncer) runSync(ctx context.Context) {
	since := s.lastSync
	now := time.Now().UTC()

	discs, err := s.discrepancyStore.ListAllResolvedSince(ctx, since)
	if err != nil {
		s.logger.Error().Err(err).Msg("RAG sync: failed to query resolved discrepancies")
		return
	}

	fed := 0
	for _, disc := range discs {
		tenantID, parseErr := uuid.Parse(disc.TenantID)
		if parseErr != nil {
			s.logger.Warn().
				Str("tenant_id", disc.TenantID).
				Msg("RAG sync: invalid tenant ID, skipping")
			continue
		}

		if err := s.feeder.FeedResolved(ctx, tenantID, disc); err != nil {
			s.logger.Warn().Err(err).
				Str("discrepancy_id", disc.ID).
				Msg("RAG sync: feed failed, continuing")
			continue
		}
		fed++
	}

	s.lastSync = now
	s.logger.Info().
		Int("total", len(discs)).
		Int("fed", fed).
		Msg("RAG sync completed")
}

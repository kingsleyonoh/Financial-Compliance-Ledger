package store

import (
	"context"
	"fmt"
	"time"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// ListAllResolvedSince returns all discrepancies across all tenants that
// have been resolved or auto-closed since the given timestamp. Used by
// the RAG syncer to find discrepancies to feed to the RAG Platform.
func (s *DiscrepancyStore) ListAllResolvedSince(
	ctx context.Context, since time.Time,
) ([]*domain.Discrepancy, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title, description, amount_expected,
			amount_actual, currency, metadata, first_detected_at,
			resolved_at, created_at, updated_at
		FROM discrepancies
		WHERE status IN ($1, $2)
		  AND resolved_at >= $3
		ORDER BY resolved_at ASC
	`, domain.StatusResolved, domain.StatusAutoClosed, since)
	if err != nil {
		return nil, fmt.Errorf(
			"discrepancy_store.ListAllResolvedSince: %w", err)
	}
	defer rows.Close()

	var results []*domain.Discrepancy
	for rows.Next() {
		d, err := scanDiscrepancyRow(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"discrepancy_store.ListAllResolvedSince: scan: %w", err)
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"discrepancy_store.ListAllResolvedSince: rows: %w", err)
	}
	return results, nil
}

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// ListForEscalation returns discrepancies matching the given status and
// severity that were created before olderThan. Used by the escalation
// engine to find candidates for rule evaluation.
// If severity is "*", all severities are matched (wildcard).
func (s *DiscrepancyStore) ListForEscalation(
	ctx context.Context, tenantID uuid.UUID,
	status string, severity string, olderThan time.Time,
) ([]*domain.Discrepancy, error) {
	query := `
		SELECT id, tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title, description, amount_expected,
			amount_actual, currency, metadata, first_detected_at,
			resolved_at, created_at, updated_at
		FROM discrepancies
		WHERE tenant_id = $1 AND status = $2 AND created_at < $3
	`
	args := []interface{}{tenantID, status, olderThan}

	if severity != "*" {
		query += " AND severity = $4"
		args = append(args, severity)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("discrepancy_store.ListForEscalation: %w", err)
	}
	defer rows.Close()

	var results []*domain.Discrepancy
	for rows.Next() {
		d, err := scanDiscrepancyRow(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"discrepancy_store.ListForEscalation: scan: %w", err)
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"discrepancy_store.ListForEscalation: rows: %w", err)
	}
	return results, nil
}

// UpdateSeverity updates the severity of a discrepancy.
func (s *DiscrepancyStore) UpdateSeverity(
	ctx context.Context, tenantID, id uuid.UUID, newSeverity string,
) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE discrepancies
		SET severity = $1, updated_at = NOW()
		WHERE tenant_id = $2 AND id = $3
	`, newSeverity, tenantID, id)
	if err != nil {
		return fmt.Errorf("discrepancy_store.UpdateSeverity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("discrepancy_store.UpdateSeverity: not found")
	}
	return nil
}

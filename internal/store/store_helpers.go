package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// DBTX is a common interface satisfied by both *pgxpool.Pool and pgx.Tx,
// enabling store methods to run inside or outside a transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows, allowing a
// single scan helper for QueryRow and Query results.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanDiscrepancyFrom scans a single discrepancy from any scanner.
func scanDiscrepancyFrom(s rowScanner) (*domain.Discrepancy, error) {
	var d domain.Discrepancy
	var metadataBytes []byte
	err := s.Scan(
		&d.ID, &d.TenantID, &d.ExternalID, &d.SourceSystem,
		&d.DiscrepancyType, &d.Severity, &d.Status, &d.Title,
		&d.Description, &d.AmountExpected, &d.AmountActual,
		&d.Currency, &metadataBytes, &d.FirstDetectedAt,
		&d.ResolvedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if metadataBytes != nil {
		_ = json.Unmarshal(metadataBytes, &d.Metadata)
	}
	return &d, nil
}

// scanDiscrepancy scans a single row from QueryRow into a Discrepancy.
func scanDiscrepancy(row pgx.Row) (*domain.Discrepancy, error) {
	return scanDiscrepancyFrom(row)
}

// scanDiscrepancyRow scans a row from Query (pgx.Rows) into a Discrepancy.
func scanDiscrepancyRow(rows pgx.Rows) (*domain.Discrepancy, error) {
	return scanDiscrepancyFrom(rows)
}

// scanRule scans a single escalation rule row from any scanner.
func scanRule(s rowScanner) (*domain.EscalationRule, error) {
	var r domain.EscalationRule
	var configOut []byte
	err := s.Scan(
		&r.ID, &r.TenantID, &r.Name, &r.SeverityMatch,
		&r.TriggerAfterHrs, &r.TriggerStatus, &r.Action,
		&configOut, &r.IsActive, &r.Priority,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if configOut != nil {
		_ = json.Unmarshal(configOut, &r.ActionConfig)
	}
	return &r, nil
}

// nilIfZero returns nil if the time is zero-value, otherwise a pointer.
func nilIfZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

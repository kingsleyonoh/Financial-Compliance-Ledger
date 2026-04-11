package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// ReportStore provides database operations for compliance reports.
type ReportStore struct {
	pool *pgxpool.Pool
}

// NewReportStore creates a new ReportStore.
func NewReportStore(pool *pgxpool.Pool) *ReportStore {
	return &ReportStore{pool: pool}
}

// Create inserts a new report and returns it with generated fields.
func (s *ReportStore) Create(
	ctx context.Context, tenantID uuid.UUID, r *domain.Report,
) (*domain.Report, error) {
	paramsBytes, err := json.Marshal(r.Parameters)
	if err != nil {
		return nil, fmt.Errorf("report_store.Create: marshal parameters: %w", err)
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO reports
			(tenant_id, report_type, title, parameters, status,
			 generated_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, report_type, title, parameters,
			status, file_path, file_size_bytes, generated_by,
			created_at, updated_at
	`, tenantID, r.ReportType, r.Title, paramsBytes,
		r.Status, r.GeneratedBy)

	return scanReport(row)
}

// GetByID fetches a report by tenant_id and id.
func (s *ReportStore) GetByID(
	ctx context.Context, tenantID uuid.UUID, id uuid.UUID,
) (*domain.Report, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, report_type, title, parameters,
			status, file_path, file_size_bytes, generated_by,
			created_at, updated_at
		FROM reports
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)

	report, err := scanReport(row)
	if err != nil {
		return nil, fmt.Errorf("report_store.GetByID: %w", err)
	}
	return report, nil
}

// List returns all reports for a tenant, ordered by created_at DESC.
func (s *ReportStore) List(
	ctx context.Context, tenantID uuid.UUID,
) ([]*domain.Report, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, report_type, title, parameters,
			status, file_path, file_size_bytes, generated_by,
			created_at, updated_at
		FROM reports
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("report_store.List: %w", err)
	}
	defer rows.Close()

	var reports []*domain.Report
	for rows.Next() {
		r, err := scanReportRow(rows)
		if err != nil {
			return nil, fmt.Errorf("report_store.List: scan: %w", err)
		}
		reports = append(reports, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("report_store.List: rows: %w", err)
	}
	return reports, nil
}

// UpdateStatus updates the status and optional file info for a report.
func (s *ReportStore) UpdateStatus(
	ctx context.Context, tenantID, id uuid.UUID,
	status string, filePath *string, fileSize *int64,
) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE reports
		SET status = $1, file_path = COALESCE($2, file_path),
			file_size_bytes = COALESCE($3, file_size_bytes),
			updated_at = NOW()
		WHERE tenant_id = $4 AND id = $5
	`, status, filePath, fileSize, tenantID, id)
	if err != nil {
		return fmt.Errorf("report_store.UpdateStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("report_store.UpdateStatus: not found")
	}
	return nil
}

// scanReport scans a single row from QueryRow into a Report.
func scanReport(row pgx.Row) (*domain.Report, error) {
	var r domain.Report
	var paramsBytes []byte
	err := row.Scan(
		&r.ID, &r.TenantID, &r.ReportType, &r.Title,
		&paramsBytes, &r.Status, &r.FilePath, &r.FileSizeBytes,
		&r.GeneratedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if paramsBytes != nil {
		_ = json.Unmarshal(paramsBytes, &r.Parameters)
	}
	return &r, nil
}

// scanReportRow scans a row from Query (pgx.Rows) into a Report.
func scanReportRow(rows pgx.Rows) (*domain.Report, error) {
	var r domain.Report
	var paramsBytes []byte
	err := rows.Scan(
		&r.ID, &r.TenantID, &r.ReportType, &r.Title,
		&paramsBytes, &r.Status, &r.FilePath, &r.FileSizeBytes,
		&r.GeneratedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if paramsBytes != nil {
		_ = json.Unmarshal(paramsBytes, &r.Parameters)
	}
	return &r, nil
}

// ListOlderThan returns all reports with created_at before the given
// time and matching the given status. Used by the report cleanup
// goroutine to find expired completed reports.
func (s *ReportStore) ListOlderThan(
	ctx context.Context, olderThan time.Time, status string,
) ([]*domain.Report, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, report_type, title, parameters,
			status, file_path, file_size_bytes, generated_by,
			created_at, updated_at
		FROM reports
		WHERE created_at < $1 AND status = $2
		ORDER BY created_at ASC
	`, olderThan, status)
	if err != nil {
		return nil, fmt.Errorf("report_store.ListOlderThan: %w", err)
	}
	defer rows.Close()

	var reports []*domain.Report
	for rows.Next() {
		r, err := scanReportRow(rows)
		if err != nil {
			return nil, fmt.Errorf("report_store.ListOlderThan: scan: %w", err)
		}
		reports = append(reports, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("report_store.ListOlderThan: rows: %w", err)
	}
	return reports, nil
}

// MarkCleaned updates a report's status to "cleaned" and clears the
// file_path. Used after the report cleanup goroutine deletes the PDF.
func (s *ReportStore) MarkCleaned(
	ctx context.Context, id uuid.UUID,
) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE reports
		SET status = 'cleaned', file_path = NULL,
			file_size_bytes = NULL, updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("report_store.MarkCleaned: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("report_store.MarkCleaned: not found")
	}
	return nil
}

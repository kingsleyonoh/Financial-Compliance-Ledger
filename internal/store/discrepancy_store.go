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

// ListFilters holds filter and pagination options for listing discrepancies.
type ListFilters struct {
	Status          string
	Severity        string
	DiscrepancyType string
	DateFrom        *time.Time
	DateTo          *time.Time
	SourceSystem    string
	Cursor          string // ID of last item from previous page
	Limit           int    // default 25, max 100
}

// DiscrepancyStore provides database operations for discrepancies.
type DiscrepancyStore struct {
	pool *pgxpool.Pool
}

// NewDiscrepancyStore creates a new DiscrepancyStore.
func NewDiscrepancyStore(pool *pgxpool.Pool) *DiscrepancyStore {
	return &DiscrepancyStore{pool: pool}
}

// Create inserts a new discrepancy and returns it with generated fields.
func (s *DiscrepancyStore) Create(
	ctx context.Context, tenantID uuid.UUID, d *domain.Discrepancy,
) (*domain.Discrepancy, error) {
	metadataBytes, err := json.Marshal(d.Metadata)
	if err != nil {
		return nil, fmt.Errorf("discrepancy_store.Create: marshal metadata: %w", err)
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO discrepancies
			(tenant_id, external_id, source_system, discrepancy_type,
			 severity, status, title, description, amount_expected,
			 amount_actual, currency, metadata, first_detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, COALESCE($13, NOW()))
		RETURNING id, tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title, description, amount_expected,
			amount_actual, currency, metadata, first_detected_at,
			resolved_at, created_at, updated_at
	`, tenantID, d.ExternalID, d.SourceSystem, d.DiscrepancyType,
		d.Severity, d.Status, d.Title, d.Description,
		d.AmountExpected, d.AmountActual, d.Currency,
		metadataBytes, nilIfZero(d.FirstDetectedAt))

	return scanDiscrepancy(row)
}

// GetByID fetches a discrepancy by tenant_id and id.
func (s *DiscrepancyStore) GetByID(
	ctx context.Context, tenantID uuid.UUID, id uuid.UUID,
) (*domain.Discrepancy, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title, description, amount_expected,
			amount_actual, currency, metadata, first_detected_at,
			resolved_at, created_at, updated_at
		FROM discrepancies
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)

	d, err := scanDiscrepancy(row)
	if err != nil {
		return nil, fmt.Errorf("discrepancy_store.GetByID: %w", err)
	}
	return d, nil
}

// GetByExternalID fetches a discrepancy by tenant_id and external_id.
func (s *DiscrepancyStore) GetByExternalID(
	ctx context.Context, tenantID uuid.UUID, externalID string,
) (*domain.Discrepancy, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title, description, amount_expected,
			amount_actual, currency, metadata, first_detected_at,
			resolved_at, created_at, updated_at
		FROM discrepancies
		WHERE tenant_id = $1 AND external_id = $2
	`, tenantID, externalID)

	d, err := scanDiscrepancy(row)
	if err != nil {
		return nil, fmt.Errorf("discrepancy_store.GetByExternalID: %w", err)
	}
	return d, nil
}

// List returns discrepancies matching the given filters with cursor-based
// pagination. Returns the results and total count.
func (s *DiscrepancyStore) List(
	ctx context.Context, tenantID uuid.UUID, filters ListFilters,
) ([]*domain.Discrepancy, int, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	// Build WHERE clauses
	args := []interface{}{tenantID}
	where := "WHERE tenant_id = $1"
	argIdx := 2

	if filters.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.Severity != "" {
		where += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, filters.Severity)
		argIdx++
	}
	if filters.DiscrepancyType != "" {
		where += fmt.Sprintf(" AND discrepancy_type = $%d", argIdx)
		args = append(args, filters.DiscrepancyType)
		argIdx++
	}
	if filters.SourceSystem != "" {
		where += fmt.Sprintf(" AND source_system = $%d", argIdx)
		args = append(args, filters.SourceSystem)
		argIdx++
	}
	if filters.DateFrom != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *filters.DateFrom)
		argIdx++
	}
	if filters.DateTo != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *filters.DateTo)
		argIdx++
	}

	// Cursor-based pagination: fetch rows after the cursor item
	if filters.Cursor != "" {
		where += fmt.Sprintf(
			" AND (created_at, id) < (SELECT created_at, id FROM discrepancies WHERE id = $%d)",
			argIdx,
		)
		args = append(args, filters.Cursor)
		argIdx++
	}

	// Count total (without cursor/limit)
	countArgs := []interface{}{tenantID}
	countWhere := "WHERE tenant_id = $1"
	countIdx := 2
	if filters.Status != "" {
		countWhere += fmt.Sprintf(" AND status = $%d", countIdx)
		countArgs = append(countArgs, filters.Status)
		countIdx++
	}
	if filters.Severity != "" {
		countWhere += fmt.Sprintf(" AND severity = $%d", countIdx)
		countArgs = append(countArgs, filters.Severity)
		countIdx++
	}
	if filters.DiscrepancyType != "" {
		countWhere += fmt.Sprintf(" AND discrepancy_type = $%d", countIdx)
		countArgs = append(countArgs, filters.DiscrepancyType)
		countIdx++
	}
	if filters.SourceSystem != "" {
		countWhere += fmt.Sprintf(" AND source_system = $%d", countIdx)
		countArgs = append(countArgs, filters.SourceSystem)
		countIdx++
	}
	if filters.DateFrom != nil {
		countWhere += fmt.Sprintf(" AND created_at >= $%d", countIdx)
		countArgs = append(countArgs, *filters.DateFrom)
		countIdx++
	}
	if filters.DateTo != nil {
		countWhere += fmt.Sprintf(" AND created_at <= $%d", countIdx)
		countArgs = append(countArgs, *filters.DateTo)
		countIdx++
	}

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM discrepancies %s", countWhere)
	err := s.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("discrepancy_store.List: count: %w", err)
	}

	// Fetch page
	query := fmt.Sprintf(`
		SELECT id, tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title, description, amount_expected,
			amount_actual, currency, metadata, first_detected_at,
			resolved_at, created_at, updated_at
		FROM discrepancies
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d
	`, where, argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("discrepancy_store.List: query: %w", err)
	}
	defer rows.Close()

	var results []*domain.Discrepancy
	for rows.Next() {
		d, err := scanDiscrepancyRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("discrepancy_store.List: scan: %w", err)
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("discrepancy_store.List: rows: %w", err)
	}

	return results, total, nil
}

// UpdateStatus updates the status and optional resolved_at timestamp.
func (s *DiscrepancyStore) UpdateStatus(
	ctx context.Context, tenantID, id uuid.UUID,
	newStatus string, resolvedAt *time.Time,
) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE discrepancies
		SET status = $1, resolved_at = $2, updated_at = NOW()
		WHERE tenant_id = $3 AND id = $4
	`, newStatus, resolvedAt, tenantID, id)
	if err != nil {
		return fmt.Errorf("discrepancy_store.UpdateStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("discrepancy_store.UpdateStatus: not found")
	}
	return nil
}

// scanDiscrepancy scans a single row from QueryRow into a Discrepancy.
func scanDiscrepancy(row pgx.Row) (*domain.Discrepancy, error) {
	var d domain.Discrepancy
	var metadataBytes []byte
	err := row.Scan(
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

// scanDiscrepancyRow scans a row from Query (pgx.Rows) into a Discrepancy.
func scanDiscrepancyRow(rows pgx.Rows) (*domain.Discrepancy, error) {
	var d domain.Discrepancy
	var metadataBytes []byte
	err := rows.Scan(
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

// nilIfZero returns nil if the time is zero-value, otherwise a pointer.
func nilIfZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// Pool returns the underlying connection pool. Used by handlers that need
// to run transactional operations spanning multiple stores.
func (s *DiscrepancyStore) Pool() *pgxpool.Pool {
	return s.pool
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

// buildListWhere builds a WHERE clause and args from ListFilters.
// When includeCursor is true the cursor condition is appended.
func buildListWhere(
	tenantID uuid.UUID, filters ListFilters, includeCursor bool,
) (string, []interface{}, int) {
	args := []interface{}{tenantID}
	where := "WHERE tenant_id = $1"
	idx := 2

	if filters.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, filters.Status)
		idx++
	}
	if filters.Severity != "" {
		where += fmt.Sprintf(" AND severity = $%d", idx)
		args = append(args, filters.Severity)
		idx++
	}
	if filters.DiscrepancyType != "" {
		where += fmt.Sprintf(" AND discrepancy_type = $%d", idx)
		args = append(args, filters.DiscrepancyType)
		idx++
	}
	if filters.SourceSystem != "" {
		where += fmt.Sprintf(" AND source_system = $%d", idx)
		args = append(args, filters.SourceSystem)
		idx++
	}
	if filters.DateFrom != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", idx)
		args = append(args, *filters.DateFrom)
		idx++
	}
	if filters.DateTo != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", idx)
		args = append(args, *filters.DateTo)
		idx++
	}
	if includeCursor && filters.Cursor != "" {
		where += fmt.Sprintf(
			" AND (created_at, id) < (SELECT created_at, id FROM discrepancies WHERE id = $%d)",
			idx,
		)
		args = append(args, filters.Cursor)
		idx++
	}
	return where, args, idx
}

// listCount returns the total number of discrepancies matching filters
// (ignoring cursor and limit).
func (s *DiscrepancyStore) listCount(
	ctx context.Context, tenantID uuid.UUID, filters ListFilters,
) (int, error) {
	where, args, _ := buildListWhere(tenantID, filters, false)
	var total int
	q := fmt.Sprintf("SELECT COUNT(*) FROM discrepancies %s", where)
	if err := s.pool.QueryRow(ctx, q, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("discrepancy_store.List: count: %w", err)
	}
	return total, nil
}

// listPage fetches a single page of discrepancies matching filters.
func (s *DiscrepancyStore) listPage(
	ctx context.Context, tenantID uuid.UUID,
	filters ListFilters, limit int,
) ([]*domain.Discrepancy, error) {
	where, args, nextIdx := buildListWhere(tenantID, filters, true)
	query := fmt.Sprintf(`
		SELECT id, tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title, description, amount_expected,
			amount_actual, currency, metadata, first_detected_at,
			resolved_at, created_at, updated_at
		FROM discrepancies
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d
	`, where, nextIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("discrepancy_store.List: query: %w", err)
	}
	defer rows.Close()

	var results []*domain.Discrepancy
	for rows.Next() {
		d, err := scanDiscrepancyRow(rows)
		if err != nil {
			return nil, fmt.Errorf("discrepancy_store.List: scan: %w", err)
		}
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("discrepancy_store.List: rows: %w", err)
	}
	return results, nil
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

	total, err := s.listCount(ctx, tenantID, filters)
	if err != nil {
		return nil, 0, err
	}

	results, err := s.listPage(ctx, tenantID, filters, limit)
	if err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// UpdateStatus updates the status and optional resolved_at timestamp.
func (s *DiscrepancyStore) UpdateStatus(
	ctx context.Context, tenantID, id uuid.UUID,
	newStatus string, resolvedAt *time.Time,
) error {
	return UpdateStatusWith(ctx, s.pool, tenantID, id, newStatus, resolvedAt)
}

// UpdateStatusWith updates status using the provided DBTX (pool or tx).
func UpdateStatusWith(
	ctx context.Context, db DBTX, tenantID, id uuid.UUID,
	newStatus string, resolvedAt *time.Time,
) error {
	tag, err := db.Exec(ctx, `
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

// GetByIDWith fetches a discrepancy using the provided DBTX (pool or tx).
func GetByIDWith(
	ctx context.Context, db DBTX, tenantID uuid.UUID, id uuid.UUID,
) (*domain.Discrepancy, error) {
	row := db.QueryRow(ctx, `
		SELECT id, tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title, description, amount_expected,
			amount_actual, currency, metadata, first_detected_at,
			resolved_at, created_at, updated_at
		FROM discrepancies
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)

	d, err := scanDiscrepancy(row)
	if err != nil {
		return nil, fmt.Errorf("discrepancy_store.GetByIDWith: %w", err)
	}
	return d, nil
}


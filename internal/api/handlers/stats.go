package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
)

// StatsHandler provides HTTP handlers for aggregate statistics.
type StatsHandler struct {
	pool *pgxpool.Pool
}

// NewStatsHandler creates a new StatsHandler.
func NewStatsHandler(pool *pgxpool.Pool) *StatsHandler {
	return &StatsHandler{pool: pool}
}

// statsResponse is the JSON response for aggregate stats.
type statsResponse struct {
	ByStatus   map[string]int `json:"by_status"`
	BySeverity map[string]int `json:"by_severity"`
	Total      int            `json:"total"`
}

// GetStats returns aggregate discrepancy counts grouped by status and severity.
func (h *StatsHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	tenantID := ctxutil.GetTenantID(r.Context())
	if tenantID == "" {
		RespondError(w, http.StatusUnauthorized,
			"MISSING_TENANT", "Tenant ID not found in context")
		return
	}

	tid, err := uuid.Parse(tenantID)
	if err != nil {
		RespondError(w, http.StatusBadRequest,
			"INVALID_TENANT", "Invalid tenant ID format")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	byStatus, err := h.countByColumn(ctx, tid, "status")
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"STATS_FAILED", "Failed to compute statistics")
		return
	}

	bySeverity, err := h.countByColumn(ctx, tid, "severity")
	if err != nil {
		RespondError(w, http.StatusInternalServerError,
			"STATS_FAILED", "Failed to compute statistics")
		return
	}

	total := 0
	for _, count := range byStatus {
		total += count
	}

	RespondJSON(w, http.StatusOK, statsResponse{
		ByStatus:   byStatus,
		BySeverity: bySeverity,
		Total:      total,
	})
}

// countByColumn queries discrepancy counts grouped by the given column.
// Column must be one of "status" or "severity" (validated by caller).
func (h *StatsHandler) countByColumn(
	ctx context.Context, tenantID uuid.UUID, column string,
) (map[string]int, error) {
	// Only allow known columns to prevent SQL injection
	if column != "status" && column != "severity" {
		return nil, fmt.Errorf("invalid column: %s", column)
	}

	query := fmt.Sprintf(
		"SELECT %s, COUNT(*) FROM discrepancies WHERE tenant_id = $1 GROUP BY %s",
		column, column,
	)

	rows, err := h.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("stats.countByColumn(%s): %w", column, err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, fmt.Errorf("stats.countByColumn(%s): scan: %w", column, err)
		}
		counts[key] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("stats.countByColumn(%s): rows: %w", column, err)
	}

	return counts, nil
}

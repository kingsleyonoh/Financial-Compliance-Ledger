package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// NotificationStore provides database operations for the notification_log
// table. Tracks outbound notifications sent via the Notification Hub.
type NotificationStore struct {
	pool *pgxpool.Pool
}

// NewNotificationStore creates a new NotificationStore.
func NewNotificationStore(pool *pgxpool.Pool) *NotificationStore {
	return &NotificationStore{pool: pool}
}

// Create inserts a new notification_log entry with status 'pending'.
// ruleID may be nil if the notification is not tied to an escalation rule.
func (s *NotificationStore) Create(
	ctx context.Context, tenantID, discrepancyID uuid.UUID,
	ruleID *uuid.UUID, channel, recipient string,
) (*domain.NotificationLog, error) {
	var n domain.NotificationLog
	var hubRespBytes []byte
	var ruleIDStr *string

	if ruleID != nil {
		s := ruleID.String()
		ruleIDStr = &s
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO notification_log
			(tenant_id, discrepancy_id, escalation_rule_id,
			 channel, recipient, status, attempts)
		VALUES ($1, $2, $3, $4, $5, 'pending', 0)
		RETURNING id, tenant_id, discrepancy_id, escalation_rule_id,
			channel, recipient, status, hub_response, attempts,
			last_attempt_at, created_at, updated_at
	`, tenantID, discrepancyID, ruleID, channel, recipient,
	).Scan(
		&n.ID, &n.TenantID, &n.DiscrepancyID, &n.EscalationRuleID,
		&n.Channel, &n.Recipient, &n.Status, &hubRespBytes,
		&n.Attempts, &n.LastAttemptAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("notification_store.Create: %w", err)
	}
	_ = ruleIDStr // ruleID passed directly to query

	if hubRespBytes != nil {
		_ = json.Unmarshal(hubRespBytes, &n.HubResponse)
	}
	return &n, nil
}

// UpdateStatus updates the status, hub_response, and attempts count
// for a notification_log entry. Also sets last_attempt_at to NOW().
func (s *NotificationStore) UpdateStatus(
	ctx context.Context, id uuid.UUID, status string,
	hubResponse map[string]interface{}, attempts int,
) error {
	var hubRespBytes []byte
	if hubResponse != nil {
		var err error
		hubRespBytes, err = json.Marshal(hubResponse)
		if err != nil {
			return fmt.Errorf("notification_store.UpdateStatus: marshal: %w", err)
		}
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE notification_log
		SET status = $1, hub_response = $2, attempts = $3,
			last_attempt_at = NOW(), updated_at = NOW()
		WHERE id = $4
	`, status, hubRespBytes, attempts, id)
	if err != nil {
		return fmt.Errorf("notification_store.UpdateStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("notification_store.UpdateStatus: not found")
	}
	return nil
}

// GetByID fetches a notification_log entry by its ID.
func (s *NotificationStore) GetByID(
	ctx context.Context, id uuid.UUID,
) (*domain.NotificationLog, error) {
	var n domain.NotificationLog
	var hubRespBytes []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, discrepancy_id, escalation_rule_id,
			channel, recipient, status, hub_response, attempts,
			last_attempt_at, created_at, updated_at
		FROM notification_log
		WHERE id = $1
	`, id).Scan(
		&n.ID, &n.TenantID, &n.DiscrepancyID, &n.EscalationRuleID,
		&n.Channel, &n.Recipient, &n.Status, &hubRespBytes,
		&n.Attempts, &n.LastAttemptAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("notification_store.GetByID: %w", err)
	}
	if hubRespBytes != nil {
		_ = json.Unmarshal(hubRespBytes, &n.HubResponse)
	}
	return &n, nil
}

// ListPendingRetries returns notification_log entries with status
// 'failed' or 'retrying' and attempts < maxAttempts.
func (s *NotificationStore) ListPendingRetries(
	ctx context.Context, maxAttempts int,
) ([]*domain.NotificationLog, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, discrepancy_id, escalation_rule_id,
			channel, recipient, status, hub_response, attempts,
			last_attempt_at, created_at, updated_at
		FROM notification_log
		WHERE status IN ('failed', 'retrying')
		  AND attempts < $1
		ORDER BY created_at ASC
	`, maxAttempts)
	if err != nil {
		return nil, fmt.Errorf("notification_store.ListPendingRetries: %w", err)
	}
	defer rows.Close()

	return scanNotificationRows(rows)
}

// GetByRuleAndDiscrepancy returns the notification_log entry for a
// specific rule+discrepancy combination. Returns nil if not found.
func (s *NotificationStore) GetByRuleAndDiscrepancy(
	ctx context.Context, ruleID, discrepancyID uuid.UUID,
) (*domain.NotificationLog, error) {
	var n domain.NotificationLog
	var hubRespBytes []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, discrepancy_id, escalation_rule_id,
			channel, recipient, status, hub_response, attempts,
			last_attempt_at, created_at, updated_at
		FROM notification_log
		WHERE escalation_rule_id = $1 AND discrepancy_id = $2
	`, ruleID, discrepancyID).Scan(
		&n.ID, &n.TenantID, &n.DiscrepancyID, &n.EscalationRuleID,
		&n.Channel, &n.Recipient, &n.Status, &hubRespBytes,
		&n.Attempts, &n.LastAttemptAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("notification_store.GetByRuleAndDiscrepancy: %w", err)
	}
	if hubRespBytes != nil {
		_ = json.Unmarshal(hubRespBytes, &n.HubResponse)
	}
	return &n, nil
}

// scanNotificationRows scans multiple rows into NotificationLog slices.
func scanNotificationRows(
	rows pgx.Rows,
) ([]*domain.NotificationLog, error) {
	var results []*domain.NotificationLog
	for rows.Next() {
		var n domain.NotificationLog
		var hubRespBytes []byte
		err := rows.Scan(
			&n.ID, &n.TenantID, &n.DiscrepancyID,
			&n.EscalationRuleID, &n.Channel, &n.Recipient,
			&n.Status, &hubRespBytes, &n.Attempts,
			&n.LastAttemptAt, &n.CreatedAt, &n.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanNotificationRows: %w", err)
		}
		if hubRespBytes != nil {
			_ = json.Unmarshal(hubRespBytes, &n.HubResponse)
		}
		results = append(results, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanNotificationRows: rows: %w", err)
	}
	return results, nil
}

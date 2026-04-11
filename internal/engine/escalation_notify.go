package engine

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/notify"
)

// executeNotify creates a notification_log entry and sends the event
// via the Notification Hub client. If the hub is disabled, the entry
// is still created with status "skipped".
func (e *EscalationEngine) executeNotify(
	ctx context.Context, tenantID uuid.UUID,
	rule *domain.EscalationRule, disc *domain.Discrepancy,
) error {
	discID, err := uuid.Parse(disc.ID)
	if err != nil {
		return fmt.Errorf("executeNotify: parse disc ID: %w", err)
	}
	ruleID, err := uuid.Parse(rule.ID)
	if err != nil {
		return fmt.Errorf("executeNotify: parse rule ID: %w", err)
	}

	// Dedup: check if this rule already fired for this discrepancy
	already, err := e.notificationExists(ctx, ruleID, discID)
	if err != nil {
		return fmt.Errorf("executeNotify: dedup check: %w", err)
	}
	if already {
		return nil
	}

	// Create notification_log entry via store
	notif, err := e.createNotificationLog(
		ctx, tenantID, discID, ruleID)
	if err != nil {
		return fmt.Errorf("executeNotify: create log: %w", err)
	}

	// Send via hub client if available
	e.sendViaHub(ctx, tenantID, rule, disc, notif)

	return nil
}

// notificationExists checks if a notification_log entry already exists
// for this rule and discrepancy combination. Uses the notification
// store if available, falls back to direct query.
func (e *EscalationEngine) notificationExists(
	ctx context.Context, ruleID, discrepancyID uuid.UUID,
) (bool, error) {
	if e.notificationStore != nil {
		n, err := e.notificationStore.GetByRuleAndDiscrepancy(
			ctx, ruleID, discrepancyID)
		if err != nil {
			return false, fmt.Errorf("notificationExists: %w", err)
		}
		return n != nil, nil
	}

	// Fallback: direct query when store not set
	var exists bool
	err := e.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM notification_log
			WHERE escalation_rule_id = $1
			  AND discrepancy_id = $2
		)
	`, ruleID, discrepancyID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("notificationExists: %w", err)
	}
	return exists, nil
}

// createNotificationLog inserts a notification_log entry. Uses the
// notification store if available, falls back to direct SQL.
func (e *EscalationEngine) createNotificationLog(
	ctx context.Context, tenantID, discID, ruleID uuid.UUID,
) (*domain.NotificationLog, error) {
	if e.notificationStore != nil {
		return e.notificationStore.Create(
			ctx, tenantID, discID, &ruleID,
			domain.ChannelInApp, "escalation-engine")
	}

	// Fallback: direct insert when store not set
	_, err := e.pool.Exec(ctx, `
		INSERT INTO notification_log
			(tenant_id, discrepancy_id, escalation_rule_id,
			 channel, recipient, status, attempts)
		VALUES ($1, $2, $3, 'in_app', 'escalation-engine', 'pending', 0)
	`, tenantID, discID, ruleID)
	if err != nil {
		return nil, err
	}
	return &domain.NotificationLog{
		TenantID:      tenantID.String(),
		DiscrepancyID: discID.String(),
		Status:        domain.NotifPending,
	}, nil
}

// sendViaHub sends the notification event via the hub client and
// updates the notification_log status accordingly.
func (e *EscalationEngine) sendViaHub(
	ctx context.Context, tenantID uuid.UUID,
	rule *domain.EscalationRule, disc *domain.Discrepancy,
	notif *domain.NotificationLog,
) {
	if e.hubClient == nil {
		e.logger.Info().
			Str("tenant_id", tenantID.String()).
			Str("discrepancy_id", disc.ID).
			Str("rule_name", rule.Name).
			Msg("notification logged (hub client not configured)")
		return
	}

	event := notify.HubEvent{
		EventType: "compliance.escalation_triggered",
		TenantID:  tenantID.String(),
		Payload: map[string]interface{}{
			"discrepancy_id": disc.ID,
			"severity":       disc.Severity,
			"status":         disc.Status,
			"title":          disc.Title,
			"rule_id":        rule.ID,
			"rule_name":      rule.Name,
		},
	}

	resp, err := e.hubClient.SendEvent(ctx, event)

	// Update notification_log if store is available
	if e.notificationStore != nil && notif.ID != "" {
		notifID, parseErr := uuid.Parse(notif.ID)
		if parseErr != nil {
			e.logger.Error().Err(parseErr).
				Msg("failed to parse notification ID")
			return
		}

		if err != nil {
			_ = e.notificationStore.UpdateStatus(
				ctx, notifID, domain.NotifFailed, nil, 1)
			e.logger.Warn().Err(err).
				Str("discrepancy_id", disc.ID).
				Msg("hub send failed, notification marked as failed")
			return
		}

		hubResp := map[string]interface{}{
			"status": resp.Status, "id": resp.ID,
		}
		status := domain.NotifSent
		if resp.Status == "skipped" {
			status = domain.NotifPending // Hub disabled
		}
		_ = e.notificationStore.UpdateStatus(
			ctx, notifID, status, hubResp, 1)
	}

	e.logger.Info().
		Str("tenant_id", tenantID.String()).
		Str("discrepancy_id", disc.ID).
		Str("rule_name", rule.Name).
		Msg("notification sent to hub")
}

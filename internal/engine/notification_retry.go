package engine

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/notify"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// NotificationRetrier periodically retries failed/retrying
// notification_log entries using the hub client.
type NotificationRetrier struct {
	notificationStore *store.NotificationStore
	hubClient         notify.NotificationSender
	maxRetries        int
	logger            zerolog.Logger
}

// NewNotificationRetrier creates a new NotificationRetrier.
func NewNotificationRetrier(
	ns *store.NotificationStore,
	hubClient notify.NotificationSender,
	maxRetries int,
	logger zerolog.Logger,
) *NotificationRetrier {
	return &NotificationRetrier{
		notificationStore: ns,
		hubClient:         hubClient,
		maxRetries:        maxRetries,
		logger: logger.With().
			Str("component", "notification-retrier").Logger(),
	}
}

// Start runs the retry loop every 5 minutes until context is cancelled.
func (r *NotificationRetrier) Start(ctx context.Context) {
	r.logger.Info().Msg("notification retrier started")

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info().Msg("notification retrier stopped")
			return
		case <-ticker.C:
			if err := r.RetryPending(ctx); err != nil {
				r.logger.Error().Err(err).
					Msg("retry cycle failed")
			}
		}
	}
}

// RetryPending queries for failed/retrying notifications with attempts
// below maxRetries, and retries each one via the hub client.
func (r *NotificationRetrier) RetryPending(
	ctx context.Context,
) error {
	pending, err := r.notificationStore.ListPendingRetries(
		ctx, r.maxRetries)
	if err != nil {
		return err
	}

	if len(pending) == 0 {
		return nil
	}

	r.logger.Info().
		Int("count", len(pending)).
		Msg("retrying pending notifications")

	for _, n := range pending {
		r.retryOne(ctx, n)
	}
	return nil
}

// retryOne attempts to re-send a single notification.
func (r *NotificationRetrier) retryOne(
	ctx context.Context, n *domain.NotificationLog,
) {
	notifID, err := uuid.Parse(n.ID)
	if err != nil {
		r.logger.Error().Err(err).
			Str("notification_id", n.ID).
			Msg("invalid notification ID, skipping")
		return
	}

	newAttempts := n.Attempts + 1

	event := notify.HubEvent{
		EventType: "compliance.escalation_triggered",
		TenantID:  n.TenantID,
		Payload: map[string]interface{}{
			"discrepancy_id":  n.DiscrepancyID,
			"notification_id": n.ID,
			"channel":         n.Channel,
			"recipient":       n.Recipient,
		},
	}

	resp, err := r.hubClient.SendEvent(ctx, event)
	if err != nil {
		// Determine status based on remaining attempts
		status := domain.NotifRetrying
		if newAttempts >= r.maxRetries {
			status = domain.NotifFailed
		}

		updateErr := r.notificationStore.UpdateStatus(
			ctx, notifID, status, nil, newAttempts)
		if updateErr != nil {
			r.logger.Error().Err(updateErr).
				Str("notification_id", n.ID).
				Msg("failed to update notification status")
		}

		r.logger.Warn().Err(err).
			Str("notification_id", n.ID).
			Int("attempts", newAttempts).
			Str("status", status).
			Msg("notification retry failed")
		return
	}

	hubResponse := map[string]interface{}{
		"status": resp.Status,
		"id":     resp.ID,
	}

	err = r.notificationStore.UpdateStatus(
		ctx, notifID, domain.NotifSent, hubResponse, newAttempts)
	if err != nil {
		r.logger.Error().Err(err).
			Str("notification_id", n.ID).
			Msg("failed to update notification status after success")
		return
	}

	r.logger.Info().
		Str("notification_id", n.ID).
		Int("attempts", newAttempts).
		Msg("notification retry succeeded")
}

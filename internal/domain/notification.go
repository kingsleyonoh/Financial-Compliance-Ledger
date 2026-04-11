package domain

import "time"

// Notification status constants for the notification_log table.
const (
	NotifPending  = "pending"
	NotifSent     = "sent"
	NotifFailed   = "failed"
	NotifRetrying = "retrying"
	NotifSkipped  = "skipped"
)

// Notification channel constants.
const (
	ChannelEmail   = "email"
	ChannelInApp   = "in_app"
	ChannelWebhook = "webhook"
)

// NotificationLog represents a row in the notification_log table.
// Tracks outbound notifications sent via the Notification Hub.
type NotificationLog struct {
	ID               string                 `json:"id"`
	TenantID         string                 `json:"tenant_id"`
	DiscrepancyID    string                 `json:"discrepancy_id"`
	EscalationRuleID *string                `json:"escalation_rule_id,omitempty"`
	Channel          string                 `json:"channel"`
	Recipient        string                 `json:"recipient"`
	Status           string                 `json:"status"`
	HubResponse      map[string]interface{} `json:"hub_response,omitempty"`
	Attempts         int                    `json:"attempts"`
	LastAttemptAt    *time.Time             `json:"last_attempt_at,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

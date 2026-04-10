-- Migration 006: Create notification_log table
-- Tracks outbound notifications sent via the Notification Hub.
-- Records delivery status, retry attempts, and hub response data.

CREATE TABLE notification_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    discrepancy_id UUID NOT NULL REFERENCES discrepancies(id),
    escalation_rule_id UUID REFERENCES escalation_rules(id),
    channel VARCHAR(20) NOT NULL CHECK (channel IN ('email', 'in_app', 'webhook')),
    recipient VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed', 'retrying')),
    hub_response JSONB,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notification_log_tenant ON notification_log(tenant_id, created_at);
CREATE INDEX idx_notification_log_status ON notification_log(status);
CREATE INDEX idx_notification_log_discrepancy ON notification_log(discrepancy_id);

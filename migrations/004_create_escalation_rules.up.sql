-- Migration 004: Create escalation_rules table
-- Configurable escalation policies per tenant.
-- Rules are data (DB rows), not code — severity_match supports wildcard '*'.
-- Unique constraint on (tenant_id, name) prevents duplicate rule names per tenant.

CREATE TABLE escalation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    severity_match VARCHAR(20) NOT NULL CHECK (severity_match IN ('low', 'medium', 'high', 'critical', '*')),
    trigger_after_hrs INTEGER NOT NULL,
    trigger_status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (trigger_status IN ('open', 'acknowledged', 'investigating')),
    action VARCHAR(20) NOT NULL CHECK (action IN ('notify', 'escalate', 'auto_close')),
    action_config JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX idx_escalation_rules_tenant ON escalation_rules(tenant_id, is_active);

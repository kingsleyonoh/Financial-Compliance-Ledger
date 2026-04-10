-- Migration 003: Create ledger_events table
-- APPEND-ONLY by design: this table must NEVER have UPDATE or DELETE operations.
-- Every discrepancy state change creates a new immutable event row.
-- sequence_num provides a monotonically increasing ordering per discrepancy.

CREATE TABLE ledger_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    discrepancy_id UUID NOT NULL REFERENCES discrepancies(id),
    event_type VARCHAR(50) NOT NULL CHECK (event_type IN (
        'discrepancy.received', 'discrepancy.acknowledged', 'discrepancy.investigation_started',
        'discrepancy.note_added', 'discrepancy.escalated', 'discrepancy.resolved', 'discrepancy.auto_closed'
    )),
    actor VARCHAR(255) NOT NULL,
    actor_type VARCHAR(20) NOT NULL DEFAULT 'system' CHECK (actor_type IN ('system', 'user', 'escalation')),
    payload JSONB NOT NULL DEFAULT '{}',
    sequence_num BIGSERIAL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ledger_events_discrepancy ON ledger_events(discrepancy_id, sequence_num);
CREATE INDEX idx_ledger_events_tenant ON ledger_events(tenant_id, created_at);
CREATE INDEX idx_ledger_events_type ON ledger_events(tenant_id, event_type);

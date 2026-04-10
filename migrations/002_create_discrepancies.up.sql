-- Migration 002: Create discrepancies table
-- Tracks financial discrepancies with type/severity/status constraints.
-- Unique constraint on (tenant_id, external_id) prevents duplicate ingestion.

CREATE TABLE discrepancies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    external_id VARCHAR(255) NOT NULL,
    source_system VARCHAR(255) NOT NULL,
    discrepancy_type VARCHAR(50) NOT NULL CHECK (discrepancy_type IN ('missing', 'mismatch', 'duplicate', 'timing')),
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'investigating', 'resolved', 'escalated', 'auto_closed')),
    title VARCHAR(500) NOT NULL,
    description TEXT,
    amount_expected DECIMAL(15,2),
    amount_actual DECIMAL(15,2),
    currency VARCHAR(3) DEFAULT 'USD',
    metadata JSONB NOT NULL DEFAULT '{}',
    first_detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, external_id)
);

CREATE INDEX idx_discrepancies_tenant_id ON discrepancies(tenant_id);
CREATE INDEX idx_discrepancies_status ON discrepancies(tenant_id, status);
CREATE INDEX idx_discrepancies_severity ON discrepancies(tenant_id, severity);
CREATE INDEX idx_discrepancies_created_at ON discrepancies(tenant_id, created_at);
CREATE INDEX idx_discrepancies_type ON discrepancies(tenant_id, discrepancy_type);

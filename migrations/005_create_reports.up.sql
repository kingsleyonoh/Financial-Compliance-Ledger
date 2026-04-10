-- Migration 005: Create reports table
-- Stores metadata for generated compliance reports (PDF).
-- Actual PDF files stored on disk at file_path.

CREATE TABLE reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    report_type VARCHAR(50) NOT NULL CHECK (report_type IN ('daily_summary', 'monthly_audit', 'discrepancy_detail', 'custom')),
    title VARCHAR(500),
    parameters JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'generating', 'completed', 'failed')),
    file_path VARCHAR(1000),
    file_size_bytes BIGINT,
    generated_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reports_tenant ON reports(tenant_id, created_at);

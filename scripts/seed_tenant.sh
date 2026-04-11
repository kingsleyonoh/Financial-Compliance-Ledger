#!/bin/bash
# Seed a test tenant into the database
# Usage: ./scripts/seed_tenant.sh "Tenant Name"

set -euo pipefail

TENANT_NAME="${1:-Test Tenant}"
DATABASE_URL="${DATABASE_URL:-postgres://fcl:localdev@localhost:5441/compliance_ledger}"

# Generate UUID
TENANT_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')

# Generate API key
RAW_KEY="fcl_live_$(openssl rand -hex 32)"

# Hash with SHA-256
HASHED_KEY=$(echo -n "$RAW_KEY" | sha256sum | cut -d' ' -f1)

# Insert into DB
psql "$DATABASE_URL" -c "INSERT INTO tenants (id, name, api_key, is_active, settings) VALUES ('$TENANT_ID', '$TENANT_NAME', '$HASHED_KEY', true, '{}');"

echo "Tenant created:"
echo "  ID: $TENANT_ID"
echo "  Name: $TENANT_NAME"
echo "  API Key: $RAW_KEY"
echo ""
echo "Use this API key in the X-API-Key header."

#!/bin/bash
# Generate and publish realistic test discrepancy events to NATS JetStream
# This is a wrapper around the Go script scripts/generate_test_events.go
#
# Usage:
#   ./scripts/generate_test_events.sh <TENANT_UUID> [COUNT] [NATS_URL]
#
# Examples:
#   ./scripts/generate_test_events.sh "550e8400-e29b-41d4-a716-446655440000"
#   ./scripts/generate_test_events.sh "550e8400-e29b-41d4-a716-446655440000" 25
#   ./scripts/generate_test_events.sh "550e8400-e29b-41d4-a716-446655440000" 10 "nats://localhost:4222"

set -euo pipefail

TENANT_ID="${1:-}"
COUNT="${2:-10}"
NATS_URL="${NATS_URL:-nats://localhost:4222}"

if [ -z "$TENANT_ID" ]; then
    echo "Error: Tenant UUID is required."
    echo ""
    echo "Usage: ./scripts/generate_test_events.sh <TENANT_UUID> [COUNT] [NATS_URL]"
    echo ""
    echo "Create a tenant first with: ./scripts/seed_tenant.sh \"My Tenant\""
    exit 1
fi

# Resolve project root (directory containing go.mod)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Publishing $COUNT test events for tenant $TENANT_ID to $NATS_URL..."
echo ""

go run "$PROJECT_ROOT/scripts/generate_test_events.go" \
    -tenant "$TENANT_ID" \
    -count "$COUNT" \
    -nats "$NATS_URL"

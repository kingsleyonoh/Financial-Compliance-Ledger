package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// EscalationEngine evaluates escalation rules on a periodic interval.
// For each active tenant, it checks all active rules against matching
// discrepancies and executes the configured action.
type EscalationEngine struct {
	ruleStore        *store.RuleStore
	discrepancyStore *store.DiscrepancyStore
	eventStore       *store.EventStore
	pool             *pgxpool.Pool
	interval         time.Duration
	logger           zerolog.Logger
}

// NewEscalationEngine creates a new EscalationEngine.
func NewEscalationEngine(
	ruleStore *store.RuleStore,
	discrepancyStore *store.DiscrepancyStore,
	eventStore *store.EventStore,
	pool *pgxpool.Pool,
	logger zerolog.Logger,
	interval time.Duration,
) *EscalationEngine {
	return &EscalationEngine{
		ruleStore:        ruleStore,
		discrepancyStore: discrepancyStore,
		eventStore:       eventStore,
		pool:             pool,
		interval:         interval,
		logger: logger.With().
			Str("component", "escalation-engine").Logger(),
	}
}

// Start runs the evaluation loop on the configured interval.
// It stops when the context is cancelled.
func (e *EscalationEngine) Start(ctx context.Context) {
	e.logger.Info().
		Dur("interval", e.interval).
		Msg("escalation engine started")

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.logger.Info().Msg("escalation engine stopped")
			return
		case <-ticker.C:
			if err := e.Evaluate(ctx); err != nil {
				e.logger.Error().Err(err).
					Msg("escalation evaluation failed")
			}
		}
	}
}

// Evaluate runs a single evaluation pass across all active tenants.
func (e *EscalationEngine) Evaluate(ctx context.Context) error {
	tenantIDs, err := e.getActiveTenantIDs(ctx)
	if err != nil {
		return fmt.Errorf("escalation.Evaluate: %w", err)
	}

	for _, tenantID := range tenantIDs {
		if err := e.evaluateTenant(ctx, tenantID); err != nil {
			e.logger.Error().Err(err).
				Str("tenant_id", tenantID.String()).
				Msg("evaluation failed for tenant, continuing")
			// Continue to next tenant — one failure shouldn't block others
		}
	}
	return nil
}

// getActiveTenantIDs returns the IDs of all active tenants.
func (e *EscalationEngine) getActiveTenantIDs(
	ctx context.Context,
) ([]uuid.UUID, error) {
	rows, err := e.pool.Query(ctx,
		`SELECT DISTINCT id FROM tenants WHERE is_active = true`)
	if err != nil {
		return nil, fmt.Errorf("getActiveTenantIDs: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("getActiveTenantIDs: scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// evaluateTenant processes all active rules for a single tenant.
func (e *EscalationEngine) evaluateTenant(
	ctx context.Context, tenantID uuid.UUID,
) error {
	rules, err := e.ruleStore.List(ctx, tenantID, true)
	if err != nil {
		return fmt.Errorf("evaluateTenant: list rules: %w", err)
	}
	if len(rules) == 0 {
		return nil
	}

	for _, rule := range rules {
		if err := e.evaluateRule(ctx, tenantID, rule); err != nil {
			e.logger.Error().Err(err).
				Str("rule_id", rule.ID).
				Str("rule_name", rule.Name).
				Msg("rule evaluation failed, continuing")
		}
	}
	return nil
}

// evaluateRule finds matching discrepancies and executes the rule action.
func (e *EscalationEngine) evaluateRule(
	ctx context.Context, tenantID uuid.UUID,
	rule *domain.EscalationRule,
) error {
	olderThan := time.Now().UTC().Add(
		-time.Duration(rule.TriggerAfterHrs) * time.Hour)

	discs, err := e.discrepancyStore.ListForEscalation(
		ctx, tenantID, rule.TriggerStatus,
		rule.SeverityMatch, olderThan)
	if err != nil {
		return fmt.Errorf("evaluateRule: list: %w", err)
	}

	for _, disc := range discs {
		if err := e.executeAction(ctx, tenantID, rule, disc); err != nil {
			e.logger.Error().Err(err).
				Str("discrepancy_id", disc.ID).
				Str("rule_id", rule.ID).
				Msg("action execution failed, continuing")
		}
	}
	return nil
}

package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// executeAction dispatches to the appropriate action handler based on
// the rule's configured action type. Each action is idempotent — it
// checks for prior execution before proceeding.
func (e *EscalationEngine) executeAction(
	ctx context.Context, tenantID uuid.UUID,
	rule *domain.EscalationRule, disc *domain.Discrepancy,
) error {
	switch rule.Action {
	case domain.ActionNotify:
		return e.executeNotify(ctx, tenantID, rule, disc)
	case domain.ActionEscalate:
		return e.executeEscalate(ctx, tenantID, rule, disc)
	case domain.ActionAutoClose:
		return e.executeAutoClose(ctx, tenantID, rule, disc)
	default:
		return fmt.Errorf("unknown action: %s", rule.Action)
	}
}

// executeEscalate changes the discrepancy status to escalated and
// appends a discrepancy.escalated event. Optionally upgrades severity
// if action_config.new_severity is set.
func (e *EscalationEngine) executeEscalate(
	ctx context.Context, tenantID uuid.UUID,
	rule *domain.EscalationRule, disc *domain.Discrepancy,
) error {
	discID, err := uuid.Parse(disc.ID)
	if err != nil {
		return fmt.Errorf("executeEscalate: parse disc ID: %w", err)
	}

	// Dedup: check if escalated event already exists
	already, err := e.eventStore.ExistsForDiscrepancy(
		ctx, tenantID, discID, domain.EventEscalated)
	if err != nil {
		return fmt.Errorf("executeEscalate: dedup check: %w", err)
	}
	if already {
		return nil
	}

	// Validate transition
	if !domain.ValidTransition(disc.Status, domain.StatusEscalated) {
		e.logger.Debug().
			Str("discrepancy_id", disc.ID).
			Str("current_status", disc.Status).
			Msg("cannot escalate: invalid transition, skipping")
		return nil
	}

	// Update severity if configured
	if newSev, ok := rule.ActionConfig["new_severity"].(string); ok {
		if err := e.discrepancyStore.UpdateSeverity(
			ctx, tenantID, discID, newSev,
		); err != nil {
			return fmt.Errorf("executeEscalate: update severity: %w", err)
		}
	}

	// Update status
	if err := e.discrepancyStore.UpdateStatus(
		ctx, tenantID, discID, domain.StatusEscalated, nil,
	); err != nil {
		return fmt.Errorf("executeEscalate: update status: %w", err)
	}

	// Append event
	event := domain.NewLedgerEvent(
		tenantID.String(), disc.ID,
		domain.EventEscalated, "escalation-engine",
		domain.ActorEscalation,
		map[string]interface{}{
			"rule_id":   rule.ID,
			"rule_name": rule.Name,
		},
	)
	if _, err := store.AppendWith(
		ctx, e.pool, tenantID, event,
	); err != nil {
		return fmt.Errorf("executeEscalate: append event: %w", err)
	}

	e.logger.Info().
		Str("tenant_id", tenantID.String()).
		Str("discrepancy_id", disc.ID).
		Str("rule_name", rule.Name).
		Msg("discrepancy escalated")

	return nil
}

// executeAutoClose sets the discrepancy status to auto_closed and
// appends a discrepancy.auto_closed event.
func (e *EscalationEngine) executeAutoClose(
	ctx context.Context, tenantID uuid.UUID,
	rule *domain.EscalationRule, disc *domain.Discrepancy,
) error {
	discID, err := uuid.Parse(disc.ID)
	if err != nil {
		return fmt.Errorf("executeAutoClose: parse disc ID: %w", err)
	}

	// Dedup: check if auto_closed event already exists
	already, err := e.eventStore.ExistsForDiscrepancy(
		ctx, tenantID, discID, domain.EventAutoClosed)
	if err != nil {
		return fmt.Errorf("executeAutoClose: dedup check: %w", err)
	}
	if already {
		return nil
	}

	// Validate transition
	if !domain.ValidTransition(disc.Status, domain.StatusAutoClosed) {
		e.logger.Debug().
			Str("discrepancy_id", disc.ID).
			Str("current_status", disc.Status).
			Msg("cannot auto-close: invalid transition, skipping")
		return nil
	}

	// Update status
	now := time.Now().UTC()
	if err := e.discrepancyStore.UpdateStatus(
		ctx, tenantID, discID, domain.StatusAutoClosed, &now,
	); err != nil {
		return fmt.Errorf("executeAutoClose: update status: %w", err)
	}

	// Append event
	event := domain.NewLedgerEvent(
		tenantID.String(), disc.ID,
		domain.EventAutoClosed, "escalation-engine",
		domain.ActorEscalation,
		map[string]interface{}{
			"rule_id":   rule.ID,
			"rule_name": rule.Name,
		},
	)
	if _, err := store.AppendWith(
		ctx, e.pool, tenantID, event,
	); err != nil {
		return fmt.Errorf("executeAutoClose: append event: %w", err)
	}

	e.logger.Info().
		Str("tenant_id", tenantID.String()).
		Str("discrepancy_id", disc.ID).
		Str("rule_name", rule.Name).
		Msg("discrepancy auto-closed")

	return nil
}

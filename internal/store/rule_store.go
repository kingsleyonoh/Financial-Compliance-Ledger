package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// RuleUpdate holds optional fields for partial rule updates.
// Only non-nil fields will be updated.
type RuleUpdate struct {
	Name            *string
	SeverityMatch   *string
	TriggerAfterHrs *int
	TriggerStatus   *string
	Action          *string
	ActionConfig    *map[string]interface{}
	IsActive        *bool
	Priority        *int
}

// RuleStore provides database operations for escalation rules.
type RuleStore struct {
	pool *pgxpool.Pool
}

// NewRuleStore creates a new RuleStore.
func NewRuleStore(pool *pgxpool.Pool) *RuleStore {
	return &RuleStore{pool: pool}
}

// Create inserts a new escalation rule and returns it with generated fields.
func (s *RuleStore) Create(
	ctx context.Context, tenantID uuid.UUID, rule *domain.EscalationRule,
) (*domain.EscalationRule, error) {
	configBytes, err := json.Marshal(rule.ActionConfig)
	if err != nil {
		return nil, fmt.Errorf("rule_store.Create: marshal config: %w", err)
	}

	var r domain.EscalationRule
	var configOut []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO escalation_rules
			(tenant_id, name, severity_match, trigger_after_hrs,
			 trigger_status, action, action_config, is_active, priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, tenant_id, name, severity_match, trigger_after_hrs,
			trigger_status, action, action_config, is_active, priority,
			created_at, updated_at
	`, tenantID, rule.Name, rule.SeverityMatch, rule.TriggerAfterHrs,
		rule.TriggerStatus, rule.Action, configBytes,
		rule.IsActive, rule.Priority,
	).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.SeverityMatch,
		&r.TriggerAfterHrs, &r.TriggerStatus, &r.Action,
		&configOut, &r.IsActive, &r.Priority,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("rule_store.Create: %w", err)
	}
	if configOut != nil {
		_ = json.Unmarshal(configOut, &r.ActionConfig)
	}
	return &r, nil
}

// GetByID fetches an escalation rule by tenant_id and id.
func (s *RuleStore) GetByID(
	ctx context.Context, tenantID uuid.UUID, id uuid.UUID,
) (*domain.EscalationRule, error) {
	var r domain.EscalationRule
	var configOut []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, severity_match, trigger_after_hrs,
			trigger_status, action, action_config, is_active, priority,
			created_at, updated_at
		FROM escalation_rules
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.SeverityMatch,
		&r.TriggerAfterHrs, &r.TriggerStatus, &r.Action,
		&configOut, &r.IsActive, &r.Priority,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("rule_store.GetByID: %w", err)
	}
	if configOut != nil {
		_ = json.Unmarshal(configOut, &r.ActionConfig)
	}
	return &r, nil
}

// List returns escalation rules for the given tenant, ordered by priority.
// If activeOnly is true, only active rules are returned.
func (s *RuleStore) List(
	ctx context.Context, tenantID uuid.UUID, activeOnly bool,
) ([]*domain.EscalationRule, error) {
	query := `
		SELECT id, tenant_id, name, severity_match, trigger_after_hrs,
			trigger_status, action, action_config, is_active, priority,
			created_at, updated_at
		FROM escalation_rules
		WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}
	if activeOnly {
		query += " AND is_active = true"
	}
	query += " ORDER BY priority ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("rule_store.List: %w", err)
	}
	defer rows.Close()

	var rules []*domain.EscalationRule
	for rows.Next() {
		var r domain.EscalationRule
		var configOut []byte
		err := rows.Scan(
			&r.ID, &r.TenantID, &r.Name, &r.SeverityMatch,
			&r.TriggerAfterHrs, &r.TriggerStatus, &r.Action,
			&configOut, &r.IsActive, &r.Priority,
			&r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("rule_store.List: scan: %w", err)
		}
		if configOut != nil {
			_ = json.Unmarshal(configOut, &r.ActionConfig)
		}
		rules = append(rules, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rule_store.List: rows: %w", err)
	}
	return rules, nil
}

// Update performs a partial update on an escalation rule. Only non-nil
// fields in RuleUpdate are changed. Returns the updated rule.
func (s *RuleStore) Update(
	ctx context.Context, tenantID uuid.UUID, id uuid.UUID, updates RuleUpdate,
) (*domain.EscalationRule, error) {
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if updates.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *updates.Name)
		argIdx++
	}
	if updates.SeverityMatch != nil {
		setClauses = append(setClauses, fmt.Sprintf("severity_match = $%d", argIdx))
		args = append(args, *updates.SeverityMatch)
		argIdx++
	}
	if updates.TriggerAfterHrs != nil {
		setClauses = append(setClauses, fmt.Sprintf("trigger_after_hrs = $%d", argIdx))
		args = append(args, *updates.TriggerAfterHrs)
		argIdx++
	}
	if updates.TriggerStatus != nil {
		setClauses = append(setClauses, fmt.Sprintf("trigger_status = $%d", argIdx))
		args = append(args, *updates.TriggerStatus)
		argIdx++
	}
	if updates.Action != nil {
		setClauses = append(setClauses, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, *updates.Action)
		argIdx++
	}
	if updates.ActionConfig != nil {
		configBytes, err := json.Marshal(*updates.ActionConfig)
		if err != nil {
			return nil, fmt.Errorf("rule_store.Update: marshal config: %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("action_config = $%d", argIdx))
		args = append(args, configBytes)
		argIdx++
	}
	if updates.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *updates.IsActive)
		argIdx++
	}
	if updates.Priority != nil {
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", argIdx))
		args = append(args, *updates.Priority)
		argIdx++
	}

	if len(setClauses) == 0 {
		// No fields to update — just return the current rule
		return s.GetByID(ctx, tenantID, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	// Add WHERE params
	args = append(args, tenantID, id)
	query := fmt.Sprintf(`
		UPDATE escalation_rules
		SET %s
		WHERE tenant_id = $%d AND id = $%d
		RETURNING id, tenant_id, name, severity_match, trigger_after_hrs,
			trigger_status, action, action_config, is_active, priority,
			created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx, argIdx+1)

	var r domain.EscalationRule
	var configOut []byte
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&r.ID, &r.TenantID, &r.Name, &r.SeverityMatch,
		&r.TriggerAfterHrs, &r.TriggerStatus, &r.Action,
		&configOut, &r.IsActive, &r.Priority,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("rule_store.Update: not found")
		}
		return nil, fmt.Errorf("rule_store.Update: %w", err)
	}
	if configOut != nil {
		_ = json.Unmarshal(configOut, &r.ActionConfig)
	}
	return &r, nil
}

// Delete removes an escalation rule by tenant_id and id.
func (s *RuleStore) Delete(
	ctx context.Context, tenantID uuid.UUID, id uuid.UUID,
) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM escalation_rules
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return fmt.Errorf("rule_store.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rule_store.Delete: not found")
	}
	return nil
}

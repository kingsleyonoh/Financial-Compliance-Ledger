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

	row := s.pool.QueryRow(ctx, `
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
	)
	r, err := scanRule(row)
	if err != nil {
		return nil, fmt.Errorf("rule_store.Create: %w", err)
	}
	return r, nil
}

// GetByID fetches an escalation rule by tenant_id and id.
func (s *RuleStore) GetByID(
	ctx context.Context, tenantID uuid.UUID, id uuid.UUID,
) (*domain.EscalationRule, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, severity_match, trigger_after_hrs,
			trigger_status, action, action_config, is_active, priority,
			created_at, updated_at
		FROM escalation_rules
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	r, err := scanRule(row)
	if err != nil {
		return nil, fmt.Errorf("rule_store.GetByID: %w", err)
	}
	return r, nil
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
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("rule_store.List: scan: %w", err)
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rule_store.List: rows: %w", err)
	}
	return rules, nil
}

// setBuilder accumulates SET clause fragments and positional args.
type setBuilder struct {
	clauses []string
	args    []interface{}
	idx     int
}

func newSetBuilder() *setBuilder { return &setBuilder{idx: 1} }

func (b *setBuilder) add(col string, val interface{}) {
	b.clauses = append(b.clauses, fmt.Sprintf("%s = $%d", col, b.idx))
	b.args = append(b.args, val)
	b.idx++
}

// buildRuleUpdateSets builds the SET clauses and args for a partial
// rule update. Returns the clauses, args, next arg index, and any error.
func buildRuleUpdateSets(
	updates RuleUpdate,
) ([]string, []interface{}, int, error) {
	b := newSetBuilder()
	if updates.Name != nil {
		b.add("name", *updates.Name)
	}
	if updates.SeverityMatch != nil {
		b.add("severity_match", *updates.SeverityMatch)
	}
	if updates.TriggerAfterHrs != nil {
		b.add("trigger_after_hrs", *updates.TriggerAfterHrs)
	}
	if updates.TriggerStatus != nil {
		b.add("trigger_status", *updates.TriggerStatus)
	}
	if updates.Action != nil {
		b.add("action", *updates.Action)
	}
	if updates.ActionConfig != nil {
		configBytes, err := json.Marshal(*updates.ActionConfig)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("marshal config: %w", err)
		}
		b.add("action_config", configBytes)
	}
	if updates.IsActive != nil {
		b.add("is_active", *updates.IsActive)
	}
	if updates.Priority != nil {
		b.add("priority", *updates.Priority)
	}
	return b.clauses, b.args, b.idx, nil
}

// Update performs a partial update on an escalation rule. Only non-nil
// fields in RuleUpdate are changed. Returns the updated rule.
func (s *RuleStore) Update(
	ctx context.Context, tenantID uuid.UUID, id uuid.UUID, updates RuleUpdate,
) (*domain.EscalationRule, error) {
	setClauses, args, nextIdx, err := buildRuleUpdateSets(updates)
	if err != nil {
		return nil, fmt.Errorf("rule_store.Update: %w", err)
	}
	if len(setClauses) == 0 {
		return s.GetByID(ctx, tenantID, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, tenantID, id)
	query := fmt.Sprintf(`
		UPDATE escalation_rules
		SET %s
		WHERE tenant_id = $%d AND id = $%d
		RETURNING id, tenant_id, name, severity_match, trigger_after_hrs,
			trigger_status, action, action_config, is_active, priority,
			created_at, updated_at
	`, strings.Join(setClauses, ", "), nextIdx, nextIdx+1)

	r, err := scanRule(s.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("rule_store.Update: not found")
		}
		return nil, fmt.Errorf("rule_store.Update: %w", err)
	}
	return r, nil
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

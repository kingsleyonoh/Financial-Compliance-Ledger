package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// ---------- Create + GetByID ----------

func TestRuleStore_CreateAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	tenantID := seedTenant(t, pool, "test-rule-create")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	rule := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            "Critical Notify",
		SeverityMatch:   domain.SeverityCritical,
		TriggerAfterHrs: 4,
		TriggerStatus:   domain.StatusOpen,
		Action:          domain.ActionNotify,
		ActionConfig:    map[string]interface{}{"channel": "#alerts"},
		IsActive:        true,
		Priority:        10,
	}

	created, err := rs.Create(ctx, tenantID, rule)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "Critical Notify", created.Name)
	assert.Equal(t, domain.SeverityCritical, created.SeverityMatch)
	assert.Equal(t, 4, created.TriggerAfterHrs)
	assert.Equal(t, domain.ActionNotify, created.Action)
	assert.True(t, created.IsActive)
	assert.Equal(t, 10, created.Priority)
	assert.False(t, created.CreatedAt.IsZero())

	// Read back
	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	fetched, err := rs.GetByID(ctx, tenantID, id)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, "Critical Notify", fetched.Name)
	assert.Equal(t, domain.SeverityCritical, fetched.SeverityMatch)
}

// ---------- List (active only / all) ----------

func TestRuleStore_List(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	tenantID := seedTenant(t, pool, "test-rule-list")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Create active rule with priority 1
	r1 := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            "Active Rule 1",
		SeverityMatch:   "*",
		TriggerAfterHrs: 8,
		TriggerStatus:   domain.StatusOpen,
		Action:          domain.ActionEscalate,
		ActionConfig:    map[string]interface{}{},
		IsActive:        true,
		Priority:        1,
	}
	_, err := rs.Create(ctx, tenantID, r1)
	require.NoError(t, err)

	// Create inactive rule with priority 0
	r2 := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            "Inactive Rule",
		SeverityMatch:   domain.SeverityLow,
		TriggerAfterHrs: 48,
		TriggerStatus:   domain.StatusOpen,
		Action:          domain.ActionAutoClose,
		ActionConfig:    map[string]interface{}{},
		IsActive:        false,
		Priority:        0,
	}
	_, err = rs.Create(ctx, tenantID, r2)
	require.NoError(t, err)

	// Create active rule with priority 5
	r3 := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            "Active Rule 5",
		SeverityMatch:   domain.SeverityHigh,
		TriggerAfterHrs: 2,
		TriggerStatus:   domain.StatusAcknowledged,
		Action:          domain.ActionNotify,
		ActionConfig:    map[string]interface{}{},
		IsActive:        true,
		Priority:        5,
	}
	_, err = rs.Create(ctx, tenantID, r3)
	require.NoError(t, err)

	// List all — should get 3
	all, err := rs.List(ctx, tenantID, false)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// List active only — should get 2, ordered by priority
	active, err := rs.List(ctx, tenantID, true)
	require.NoError(t, err)
	require.Len(t, active, 2)
	// Should be ordered by priority ASC
	assert.LessOrEqual(t, active[0].Priority, active[1].Priority)
}

// ---------- Update (partial) ----------

func TestRuleStore_Update(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	tenantID := seedTenant(t, pool, "test-rule-update")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	rule := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            "Update Test Rule",
		SeverityMatch:   domain.SeverityMedium,
		TriggerAfterHrs: 12,
		TriggerStatus:   domain.StatusOpen,
		Action:          domain.ActionNotify,
		ActionConfig:    map[string]interface{}{"channel": "#original"},
		IsActive:        true,
		Priority:        5,
	}
	created, err := rs.Create(ctx, tenantID, rule)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	// Partial update: change name and priority only
	newName := "Updated Rule Name"
	newPriority := 1
	updated, err := rs.Update(ctx, tenantID, id, store.RuleUpdate{
		Name:     &newName,
		Priority: &newPriority,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "Updated Rule Name", updated.Name)
	assert.Equal(t, 1, updated.Priority)
	// Unchanged fields should remain
	assert.Equal(t, domain.SeverityMedium, updated.SeverityMatch)
	assert.Equal(t, 12, updated.TriggerAfterHrs)
	assert.Equal(t, domain.ActionNotify, updated.Action)
	assert.True(t, updated.IsActive)
}

// ---------- Delete ----------

func TestRuleStore_Delete(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	tenantID := seedTenant(t, pool, "test-rule-delete")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	rule := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            "Delete Test Rule",
		SeverityMatch:   domain.SeverityLow,
		TriggerAfterHrs: 24,
		TriggerStatus:   domain.StatusOpen,
		Action:          domain.ActionAutoClose,
		ActionConfig:    map[string]interface{}{},
		IsActive:        true,
		Priority:        0,
	}
	created, err := rs.Create(ctx, tenantID, rule)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	// Delete
	err = rs.Delete(ctx, tenantID, id)
	require.NoError(t, err)

	// Should not be found
	fetched, err := rs.GetByID(ctx, tenantID, id)
	assert.Error(t, err)
	assert.Nil(t, fetched)
}

// ---------- Unique Name Constraint ----------

func TestRuleStore_UniqueNamePerTenant(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	tenantID := seedTenant(t, pool, "test-rule-uniq")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	rule := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            "Unique Name Rule",
		SeverityMatch:   "*",
		TriggerAfterHrs: 1,
		TriggerStatus:   domain.StatusOpen,
		Action:          domain.ActionNotify,
		ActionConfig:    map[string]interface{}{},
		IsActive:        true,
		Priority:        0,
	}
	_, err := rs.Create(ctx, tenantID, rule)
	require.NoError(t, err)

	// Duplicate name for same tenant should fail
	_, err = rs.Create(ctx, tenantID, rule)
	require.Error(t, err, "duplicate rule name should fail")
}

// ---------- Tenant Isolation ----------

func TestRuleStore_TenantIsolation(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)

	tenantA := seedTenant(t, pool, "tenant-A-rule")
	tenantB := seedTenant(t, pool, "tenant-B-rule")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantA, tenantB) })

	ctx := context.Background()

	rule := &domain.EscalationRule{
		TenantID:        tenantA.String(),
		Name:            "Tenant A Rule",
		SeverityMatch:   "*",
		TriggerAfterHrs: 1,
		TriggerStatus:   domain.StatusOpen,
		Action:          domain.ActionNotify,
		ActionConfig:    map[string]interface{}{},
		IsActive:        true,
		Priority:        0,
	}
	created, err := rs.Create(ctx, tenantA, rule)
	require.NoError(t, err)

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	// Tenant B should NOT see tenant A's rule
	fetched, err := rs.GetByID(ctx, tenantB, id)
	assert.Error(t, err)
	assert.Nil(t, fetched)

	// Tenant B's list should be empty
	rules, err := rs.List(ctx, tenantB, false)
	require.NoError(t, err)
	assert.Empty(t, rules)
}

// ---------- GetByID Not Found ----------

func TestRuleStore_GetByID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	tenantID := seedTenant(t, pool, "test-rule-notfound")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	nonExistent := uuid.New()
	fetched, err := rs.GetByID(ctx, tenantID, nonExistent)
	assert.Error(t, err)
	assert.Nil(t, fetched)
}

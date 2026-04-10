package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------- Report Type Constants ----------

func TestReportTypeConstants(t *testing.T) {
	assert.Equal(t, "daily_summary", ReportTypeDailySummary)
	assert.Equal(t, "monthly_audit", ReportTypeMonthlyAudit)
	assert.Equal(t, "discrepancy_detail", ReportTypeDiscrepancyDetail)
	assert.Equal(t, "custom", ReportTypeCustom)
}

// ---------- Report Status Constants ----------

func TestReportStatusConstants(t *testing.T) {
	assert.Equal(t, "pending", ReportStatusPending)
	assert.Equal(t, "generating", ReportStatusGenerating)
	assert.Equal(t, "completed", ReportStatusCompleted)
	assert.Equal(t, "failed", ReportStatusFailed)
}

// ---------- ValidReportType ----------

func TestValidReportType_AllValid(t *testing.T) {
	validTypes := []string{
		ReportTypeDailySummary,
		ReportTypeMonthlyAudit,
		ReportTypeDiscrepancyDetail,
		ReportTypeCustom,
	}
	for _, rt := range validTypes {
		assert.True(t, ValidReportType(rt), "%s should be a valid report type", rt)
	}
}

func TestValidReportType_Invalid(t *testing.T) {
	invalidTypes := []string{
		"",
		"weekly",
		"DAILY_SUMMARY",
		"annual_audit",
		"unknown",
	}
	for _, rt := range invalidTypes {
		assert.False(t, ValidReportType(rt), "%q should be an invalid report type", rt)
	}
}

// ---------- ValidReportStatus ----------

func TestValidReportStatus_AllValid(t *testing.T) {
	validStatuses := []string{
		ReportStatusPending,
		ReportStatusGenerating,
		ReportStatusCompleted,
		ReportStatusFailed,
	}
	for _, rs := range validStatuses {
		assert.True(t, ValidReportStatus(rs), "%s should be a valid report status", rs)
	}
}

func TestValidReportStatus_Invalid(t *testing.T) {
	invalidStatuses := []string{
		"",
		"queued",
		"PENDING",
		"cancelled",
		"unknown",
	}
	for _, rs := range invalidStatuses {
		assert.False(t, ValidReportStatus(rs), "%q should be an invalid report status", rs)
	}
}

// ---------- Report Struct ----------

func TestReportStruct_HasRequiredFields(t *testing.T) {
	params := map[string]interface{}{"start_date": "2026-01-01"}
	r := Report{
		ID:         "report-id",
		TenantID:   "tenant-id",
		ReportType: ReportTypeDailySummary,
		Title:      "Daily Summary Report",
		Parameters: params,
		Status:     ReportStatusPending,
	}

	assert.Equal(t, "report-id", r.ID)
	assert.Equal(t, "tenant-id", r.TenantID)
	assert.Equal(t, ReportTypeDailySummary, r.ReportType)
	assert.Equal(t, "Daily Summary Report", r.Title)
	assert.Equal(t, params, r.Parameters)
	assert.Equal(t, ReportStatusPending, r.Status)
}

func TestReportStruct_OptionalFields(t *testing.T) {
	r := Report{}
	assert.Nil(t, r.FilePath)
	assert.Nil(t, r.FileSizeBytes)
	assert.Nil(t, r.GeneratedBy)
	assert.Nil(t, r.Parameters)
}

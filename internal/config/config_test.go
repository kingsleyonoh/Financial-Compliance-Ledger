package config_test

import (
	"os"
	"testing"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearConfigEnvVars unsets all env vars used by config.Load so tests start
// from a clean state. Call this in every test to avoid cross-test pollution.
func clearConfigEnvVars(t *testing.T) {
	t.Helper()
	vars := []string{
		"DATABASE_URL", "NATS_URL", "NATS_TOKEN", "NATS_SUBJECT",
		"NATS_CONSUMER_NAME", "PORT", "LOG_LEVEL",
		"NOTIFICATION_HUB_URL", "NOTIFICATION_HUB_API_KEY",
		"NOTIFICATION_HUB_ENABLED",
		"RAG_PLATFORM_URL", "RAG_PLATFORM_API_KEY", "RAG_FEED_ENABLED",
		"ESCALATION_INTERVAL_MINUTES", "MAX_NOTIFICATION_RETRIES",
		"SELF_REGISTRATION_ENABLED", "REPORT_STORAGE_PATH",
		"REPORT_MAX_EVENTS",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	clearConfigEnvVars(t)

	cfg := config.Load()

	// String defaults
	assert.Equal(t, "", cfg.DatabaseURL, "DATABASE_URL should default to empty")
	assert.Equal(t, "", cfg.NATSURL, "NATS_URL should default to empty")
	assert.Equal(t, "", cfg.NATSToken, "NATS_TOKEN should default to empty")
	assert.Equal(t, "recon.discrepancy.detected", cfg.NATSSubject)
	assert.Equal(t, "compliance-ledger", cfg.NATSConsumerName)
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "info", cfg.LogLevel)

	// Notification Hub defaults
	assert.Equal(t, "", cfg.NotificationHubURL)
	assert.Equal(t, "", cfg.NotificationHubAPIKey)
	assert.False(t, cfg.NotificationHubEnabled)

	// RAG Platform defaults
	assert.Equal(t, "", cfg.RAGPlatformURL)
	assert.Equal(t, "", cfg.RAGPlatformAPIKey)
	assert.False(t, cfg.RAGFeedEnabled)

	// Numeric defaults
	assert.Equal(t, 15, cfg.EscalationIntervalMinutes)
	assert.Equal(t, 3, cfg.MaxNotificationRetries)

	// Bool defaults
	assert.True(t, cfg.SelfRegistrationEnabled)

	// Report defaults
	assert.Equal(t, "data/reports", cfg.ReportStoragePath)
	assert.Equal(t, 10000, cfg.ReportMaxEvents)
}

func TestLoad_EnvVarsOverrideDefaults(t *testing.T) {
	clearConfigEnvVars(t)

	t.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")
	t.Setenv("NATS_URL", "nats://nats:4222")
	t.Setenv("NATS_TOKEN", "secret-token")
	t.Setenv("NATS_SUBJECT", "custom.subject")
	t.Setenv("NATS_CONSUMER_NAME", "custom-consumer")
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("NOTIFICATION_HUB_URL", "https://notify.example.com")
	t.Setenv("NOTIFICATION_HUB_API_KEY", "hub-key-123")
	t.Setenv("RAG_PLATFORM_URL", "https://rag.example.com")
	t.Setenv("RAG_PLATFORM_API_KEY", "rag-key-456")
	t.Setenv("REPORT_STORAGE_PATH", "/var/reports")

	cfg := config.Load()

	assert.Equal(t, "postgres://user:pass@host:5432/db", cfg.DatabaseURL)
	assert.Equal(t, "nats://nats:4222", cfg.NATSURL)
	assert.Equal(t, "secret-token", cfg.NATSToken)
	assert.Equal(t, "custom.subject", cfg.NATSSubject)
	assert.Equal(t, "custom-consumer", cfg.NATSConsumerName)
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "https://notify.example.com", cfg.NotificationHubURL)
	assert.Equal(t, "hub-key-123", cfg.NotificationHubAPIKey)
	assert.Equal(t, "https://rag.example.com", cfg.RAGPlatformURL)
	assert.Equal(t, "rag-key-456", cfg.RAGPlatformAPIKey)
	assert.Equal(t, "/var/reports", cfg.ReportStoragePath)
}

func TestLoad_BoolParsing_True(t *testing.T) {
	clearConfigEnvVars(t)

	t.Setenv("NOTIFICATION_HUB_ENABLED", "true")
	t.Setenv("RAG_FEED_ENABLED", "true")
	t.Setenv("SELF_REGISTRATION_ENABLED", "true")

	cfg := config.Load()

	assert.True(t, cfg.NotificationHubEnabled)
	assert.True(t, cfg.RAGFeedEnabled)
	assert.True(t, cfg.SelfRegistrationEnabled)
}

func TestLoad_BoolParsing_False(t *testing.T) {
	clearConfigEnvVars(t)

	t.Setenv("NOTIFICATION_HUB_ENABLED", "false")
	t.Setenv("RAG_FEED_ENABLED", "false")
	t.Setenv("SELF_REGISTRATION_ENABLED", "false")

	cfg := config.Load()

	assert.False(t, cfg.NotificationHubEnabled)
	assert.False(t, cfg.RAGFeedEnabled)
	assert.False(t, cfg.SelfRegistrationEnabled)
}

func TestLoad_BoolParsing_InvalidDefaultsToFalse(t *testing.T) {
	clearConfigEnvVars(t)

	t.Setenv("NOTIFICATION_HUB_ENABLED", "yes")
	t.Setenv("RAG_FEED_ENABLED", "nope")

	cfg := config.Load()

	// Invalid bool strings (not recognized by strconv.ParseBool) default to false
	assert.False(t, cfg.NotificationHubEnabled)
	assert.False(t, cfg.RAGFeedEnabled)
}

func TestLoad_BoolParsing_NumericTrue(t *testing.T) {
	clearConfigEnvVars(t)

	// strconv.ParseBool accepts "1" as true and "0" as false
	t.Setenv("NOTIFICATION_HUB_ENABLED", "1")
	t.Setenv("RAG_FEED_ENABLED", "0")

	cfg := config.Load()

	assert.True(t, cfg.NotificationHubEnabled)
	assert.False(t, cfg.RAGFeedEnabled)
}

func TestLoad_IntParsing_ValidValues(t *testing.T) {
	clearConfigEnvVars(t)

	t.Setenv("ESCALATION_INTERVAL_MINUTES", "30")
	t.Setenv("MAX_NOTIFICATION_RETRIES", "5")
	t.Setenv("REPORT_MAX_EVENTS", "50000")

	cfg := config.Load()

	assert.Equal(t, 30, cfg.EscalationIntervalMinutes)
	assert.Equal(t, 5, cfg.MaxNotificationRetries)
	assert.Equal(t, 50000, cfg.ReportMaxEvents)
}

func TestLoad_IntParsing_InvalidFallsBackToDefault(t *testing.T) {
	clearConfigEnvVars(t)

	t.Setenv("ESCALATION_INTERVAL_MINUTES", "not-a-number")
	t.Setenv("MAX_NOTIFICATION_RETRIES", "abc")
	t.Setenv("REPORT_MAX_EVENTS", "xyz")

	cfg := config.Load()

	assert.Equal(t, 15, cfg.EscalationIntervalMinutes)
	assert.Equal(t, 3, cfg.MaxNotificationRetries)
	assert.Equal(t, 10000, cfg.ReportMaxEvents)
}

func TestLoad_SelfRegistrationEnabled_DefaultTrue(t *testing.T) {
	clearConfigEnvVars(t)

	cfg := config.Load()

	// SELF_REGISTRATION_ENABLED defaults to true (unlike other bools)
	assert.True(t, cfg.SelfRegistrationEnabled)
}

func TestLoad_PortDefault(t *testing.T) {
	clearConfigEnvVars(t)

	cfg := config.Load()

	assert.Equal(t, "8080", cfg.Port, "PORT should default to 8080")
}

func TestConfig_FieldTypes(t *testing.T) {
	clearConfigEnvVars(t)

	cfg := config.Load()
	require.NotNil(t, cfg)

	// Verify the struct has the expected types by exercising them
	var _ string = cfg.DatabaseURL
	var _ string = cfg.NATSURL
	var _ string = cfg.Port
	var _ bool = cfg.NotificationHubEnabled
	var _ bool = cfg.RAGFeedEnabled
	var _ bool = cfg.SelfRegistrationEnabled
	var _ int = cfg.EscalationIntervalMinutes
	var _ int = cfg.MaxNotificationRetries
	var _ int = cfg.ReportMaxEvents
}

// Package config handles loading and parsing of environment variables
// for the Financial Compliance Ledger service.
package config

import (
	"os"
	"strconv"
)

// Config holds all configuration values for the application.
// Values are loaded from environment variables with sensible defaults.
type Config struct {
	// Database
	DatabaseURL string

	// NATS
	NATSURL          string
	NATSToken        string
	NATSSubject      string
	NATSConsumerName string

	// Server
	Port     string
	LogLevel string

	// Notification Hub
	NotificationHubURL     string
	NotificationHubAPIKey  string
	NotificationHubEnabled bool

	// RAG Platform
	RAGPlatformURL    string
	RAGPlatformAPIKey string
	RAGFeedEnabled    bool

	// Escalation
	EscalationIntervalMinutes int
	MaxNotificationRetries    int

	// Tenant
	SelfRegistrationEnabled bool

	// Reports
	ReportStoragePath string
	ReportMaxEvents   int
}

// Load reads configuration from environment variables with defaults.
// It does not return an error — invalid numeric/bool values fall back
// to their defaults silently (logged at startup by the caller).
func Load() Config {
	return Config{
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		NATSURL:          getEnv("NATS_URL", ""),
		NATSToken:        getEnv("NATS_TOKEN", ""),
		NATSSubject:      getEnv("NATS_SUBJECT", "recon.discrepancy.detected"),
		NATSConsumerName: getEnv("NATS_CONSUMER_NAME", "compliance-ledger"),

		Port:     getEnv("PORT", "8080"),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		NotificationHubURL:     getEnv("NOTIFICATION_HUB_URL", ""),
		NotificationHubAPIKey:  getEnv("NOTIFICATION_HUB_API_KEY", ""),
		NotificationHubEnabled: getEnvBool("NOTIFICATION_HUB_ENABLED", false),

		RAGPlatformURL:    getEnv("RAG_PLATFORM_URL", ""),
		RAGPlatformAPIKey: getEnv("RAG_PLATFORM_API_KEY", ""),
		RAGFeedEnabled:    getEnvBool("RAG_FEED_ENABLED", false),

		EscalationIntervalMinutes: getEnvInt("ESCALATION_INTERVAL_MINUTES", 15),
		MaxNotificationRetries:    getEnvInt("MAX_NOTIFICATION_RETRIES", 3),

		SelfRegistrationEnabled: getEnvBool("SELF_REGISTRATION_ENABLED", true),

		ReportStoragePath: getEnv("REPORT_STORAGE_PATH", "data/reports"),
		ReportMaxEvents:   getEnvInt("REPORT_MAX_EVENTS", 10000),
	}
}

// getEnv returns the value of an environment variable or a default if not set.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt parses an environment variable as an integer.
// Returns the fallback if the variable is not set or cannot be parsed.
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// getEnvBool parses an environment variable as a boolean.
// Only "true" (case-sensitive) is considered true.
// Returns the fallback if the variable is not set or cannot be parsed.
func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

// Package main is the entry point for the Financial Compliance Ledger server.
// It wires all dependencies and starts the HTTP server with graceful shutdown.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
	mw "github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/middleware"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/notify"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/report"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

func main() {
	cfg := config.Load()
	logger := newLogger(cfg.LogLevel)

	logger.Info().Msg("starting Financial Compliance Ledger server")

	pool, err := initDatabase(cfg, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize database")
	}
	defer pool.Close()

	healthHandler := handlers.NewHealthHandler(pool, cfg.NATSURL)
	tenantHandler := handlers.NewTenantHandler(pool, &cfg)

	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	rs := store.NewRuleStore(pool)
	rptStore := store.NewReportStore(pool)
	discrepancyHandler := handlers.NewDiscrepancyHandler(ds, es)
	rulesHandler := handlers.NewRulesHandler(rs)
	statsHandler := handlers.NewStatsHandler(pool)

	// Ensure report storage directory exists
	if err := os.MkdirAll(cfg.ReportStoragePath, 0o755); err != nil {
		logger.Warn().Err(err).Msg("failed to create report storage directory")
	}

	reportGen := report.NewReportGenerator(
		ds, es, rptStore, cfg.ReportStoragePath, cfg.ReportMaxEvents, logger)
	reportsHandler := handlers.NewReportsHandler(rptStore, reportGen)

	router := api.NewRouter(api.RouterDeps{
		Pool:               pool,
		Logger:             logger,
		Config:             &cfg,
		HealthHandler:      healthHandler,
		TenantHandler:      tenantHandler,
		DiscrepancyHandler: discrepancyHandler,
		RulesHandler:       rulesHandler,
		StatsHandler:       statsHandler,
		ReportsHandler:     reportsHandler,
	})

	// Notification Hub client
	hubClient := notify.NewHubClient(&cfg, logger)
	ns := store.NewNotificationStore(pool)

	// Start escalation engine with notification support
	escalationInterval := time.Duration(cfg.EscalationIntervalMinutes) * time.Minute
	escEngine := engine.NewEscalationEngine(
		rs, ds, es, pool, logger, escalationInterval)
	escEngine.WithNotifications(ns, hubClient)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go escEngine.Start(ctx)

	// Start notification retry goroutine
	retrier := engine.NewNotificationRetrier(
		ns, hubClient, cfg.MaxNotificationRetries, logger)
	go retrier.Start(ctx)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go startServer(srv, logger)

	waitForShutdown(srv, logger)
}

// newLogger creates a zerolog.Logger based on the configured log level.
func newLogger(level string) zerolog.Logger {
	return mw.NewLogger(level)
}

// initDatabase creates the PostgreSQL pool and runs migrations.
func initDatabase(
	cfg config.Config, logger zerolog.Logger,
) (*pgxpool.Pool, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	logger.Info().Msg("connecting to PostgreSQL")
	pool, err := store.NewPostgresPool(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}

	logger.Info().Msg("running database migrations")
	if err := store.RunMigrations(cfg.DatabaseURL, "file://migrations"); err != nil {
		pool.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	logger.Info().Msg("database ready")
	return pool, nil
}

// startServer starts the HTTP server and logs any fatal errors.
func startServer(srv *http.Server, logger zerolog.Logger) {
	logger.Info().
		Str("addr", srv.Addr).
		Msg("HTTP server listening")

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal().Err(err).Msg("server failed")
	}
}

// waitForShutdown blocks until a SIGINT or SIGTERM is received,
// then gracefully shuts down the server with a 30-second timeout.
func waitForShutdown(srv *http.Server, logger zerolog.Logger) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	logger.Info().
		Str("signal", sig.String()).
		Msg("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msg("server forced to shutdown")
	}

	logger.Info().Msg("server stopped gracefully")
}

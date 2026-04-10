// Package helpers provides test utilities for E2E tests, including
// server lifecycle management (build, start, health check, stop).
package helpers

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// ServerConfig holds configuration for starting the test server.
type ServerConfig struct {
	// Port the server listens on (default: 18080 to avoid conflicts).
	Port string
	// DatabaseURL for the test database.
	DatabaseURL string
	// NATSURL for the test NATS instance.
	NATSURL string
	// BinaryPath overrides the default binary location.
	BinaryPath string
}

// DefaultConfig returns a ServerConfig with sensible test defaults.
func DefaultConfig() ServerConfig {
	return ServerConfig{
		Port:        envOrDefault("TEST_PORT", "18080"),
		DatabaseURL: envOrDefault("DATABASE_URL", "postgres://fcl:localdev@localhost:5441/compliance_ledger"),
		NATSURL:     envOrDefault("NATS_URL", "nats://localhost:4222"),
	}
}

// TestServer manages the lifecycle of a server binary for E2E testing.
type TestServer struct {
	config     ServerConfig
	cmd        *exec.Cmd
	binaryPath string
	BaseURL    string
}

// NewTestServer creates a new TestServer with the given config.
func NewTestServer(cfg ServerConfig) *TestServer {
	if cfg.Port == "" {
		cfg.Port = "18080"
	}

	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		binaryPath = defaultBinaryPath()
	}

	return &TestServer{
		config:     cfg,
		binaryPath: binaryPath,
		BaseURL:    fmt.Sprintf("http://localhost:%s", cfg.Port),
	}
}

// Build compiles the server binary from cmd/server/main.go.
// Returns an error if the build fails.
func (s *TestServer) Build() error {
	projectRoot := projectRootPath()
	mainPath := filepath.Join(projectRoot, "cmd", "server", "main.go")

	// Check if main.go exists before attempting to build
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		return fmt.Errorf("server source not found at %s: server not yet implemented", mainPath)
	}

	cmd := exec.Command("go", "build", "-o", s.binaryPath, mainPath)
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build server binary: %w", err)
	}

	return nil
}

// Start launches the server binary in the background and waits for
// it to become healthy. Returns an error if the server fails to start
// or doesn't pass the health check within the timeout.
func (s *TestServer) Start() error {
	if _, err := os.Stat(s.binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("server binary not found at %s: run Build() first", s.binaryPath)
	}

	s.cmd = exec.Command(s.binaryPath)
	s.cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%s", s.config.Port),
		fmt.Sprintf("DATABASE_URL=%s", s.config.DatabaseURL),
		fmt.Sprintf("NATS_URL=%s", s.config.NATSURL),
		"LOG_LEVEL=error",
	)
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	if err := s.waitForHealth(10 * time.Second); err != nil {
		s.Stop()
		return fmt.Errorf("server health check: %w", err)
	}

	return nil
}

// Stop terminates the running server process.
func (s *TestServer) Stop() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}

	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
}

// waitForHealth polls the server's health endpoint until it responds
// with 200 OK or the timeout expires.
func (s *TestServer) waitForHealth(timeout time.Duration) error {
	healthURL := fmt.Sprintf("%s/health", s.BaseURL)
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("server at %s did not become healthy within %v", healthURL, timeout)
}

// projectRootPath returns the project root directory by walking up from
// this file's location until it finds go.mod.
func projectRootPath() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// This file is at tests/e2e/helpers/server.go
	// Project root is 3 levels up
	dir := filepath.Dir(filename)
	return filepath.Join(dir, "..", "..", "..")
}

// defaultBinaryPath returns the default location for the compiled server binary.
func defaultBinaryPath() string {
	root := projectRootPath()
	if runtime.GOOS == "windows" {
		return filepath.Join(root, "bin", "server.exe")
	}
	return filepath.Join(root, "bin", "server")
}

// envOrDefault returns the value of an environment variable or a default.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

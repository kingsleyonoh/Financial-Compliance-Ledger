package middleware

import (
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// NewLogger creates a structured zerolog.Logger configured for the given level.
// Valid levels: "debug", "info", "warn", "error". Defaults to "info".
// In development, output is formatted with console writer for readability.
func NewLogger(level string) zerolog.Logger {
	lvl := parseLevel(level)

	return zerolog.New(
		zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		},
	).Level(lvl).With().Timestamp().Logger()
}

// NewProductionLogger creates a JSON-formatted zerolog.Logger for production use.
func NewProductionLogger(level string) zerolog.Logger {
	lvl := parseLevel(level)

	return zerolog.New(os.Stdout).
		Level(lvl).
		With().
		Timestamp().
		Logger()
}

// parseLevel converts a string log level to a zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

// RequestLogger returns middleware that logs each HTTP request with
// method, path, status code, duration, and optional tenant_id/request_id.
func RequestLogger(logger zerolog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			duration := time.Since(start)

			event := logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", sw.status).
				Float64("duration_ms", float64(duration.Nanoseconds())/1e6)

			// Include tenant_id if present in context
			if tenantID := GetTenantID(r.Context()); tenantID != "" {
				event = event.Str("tenant_id", tenantID)
			}

			// Include request_id if present in context
			if requestID := GetRequestID(r.Context()); requestID != "" {
				event = event.Str("request_id", requestID)
			}

			event.Msg("http request")
		})
	}
}

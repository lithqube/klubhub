package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// RequestLogger returns a middleware that:
//   - Generates a UUID request ID and stores it in the request context.
//   - Sets the X-Request-ID response header.
//   - Logs method, path, status, and duration upon completion using zerolog.
func RequestLogger(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := uuid.New().String()
			w.Header().Set("X-Request-ID", requestID)

			// Store in context for downstream handlers
			ctx := r.Context()
			r = r.WithContext(ctx)

			// Wrap the ResponseWriter to capture the status code
			wrapped := &statusRecorder{ResponseWriter: w, code: http.StatusOK}

			start := time.Now()
			next.ServeHTTP(wrapped, r)
			duration := time.Since(start)

			log.Info().
				Str("request_id", requestID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", wrapped.code).
				Dur("duration_ms", duration).
				Msg("request completed")
		})
	}
}

// statusRecorder wraps http.ResponseWriter to capture the written HTTP status code.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.code = code
	sr.ResponseWriter.WriteHeader(code)
}

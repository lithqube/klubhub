package http

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// MaxBodyBytes is the default request body cap applied to all routes
// unless overridden via MaxBytesHandlerWithLimit. Plan B.6 picked
// 50 MiB because the tracklist upload legitimately accepts that size.
// Individual JSON and media handlers apply tighter route-specific caps;
// this is the absolute outer ceiling.
const MaxBodyBytes int64 = 50 << 20 // 50 MiB — largest supported upload

// MaxBodyBytesMiddleware caps every request body at MaxBodyBytes and
// ensures the response is 413 if the cap is exceeded.
//
// The middleware wraps r.Body in http.MaxBytesReader. If the body is
// short enough, downstream handlers behave unchanged. If it exceeds
// the cap, the read fails with *http.MaxBytesError; handlers must
// detect that error and return 413 (see DetectMaxBytes413).
//
// For obvious cases (Content-Length declared), the middleware short-
// circuits with 413 directly and never invokes the handler.
func MaxBodyBytesMiddleware(next http.Handler) http.Handler {
	return MaxBytesHandlerWithLimit(next, MaxBodyBytes)
}

// MaxBytesHandlerWithLimit is the explicit-limit variant exposed for
// tests and routes that need a different default.
func MaxBytesHandlerWithLimit(next http.Handler, n int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip bodies for GET/HEAD/DELETE which conventionally have no
		// body — keep the wrapper out of the way for those.
		if r.Body == nil || (r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodDelete) {
			next.ServeHTTP(w, r)
			return
		}

		// Cheap pre-check: declared Content-Length over the cap fails
		// fast with 413 before any allocation.
		if r.ContentLength > n {
			http.Error(w, "request body exceeds limit", http.StatusRequestEntityTooLarge)
			return
		}

		// Wrap the body so chunked uploads can never bypass the cap.
		r.Body = http.MaxBytesReader(w, r.Body, n)

		next.ServeHTTP(w, r)
	})
}

// DetectMaxBytes413 inspects err from a body read and returns the
// HTTP status code the caller should serve: 413 if the read failed
// because of MaxBytesReader cap overflow, 400 otherwise (parser
// errors on truncated bodies are typically due to client error,
// not attack).
//
// Handlers that decode JSON or multipart bodies should call this
// when their reader returns an error so the middleware's cap becomes
// observable as 413 in production.
func DetectMaxBytes413(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

// keep chiMiddleware import live for future router wiring use.
var _ = middleware.NewWrapResponseWriter

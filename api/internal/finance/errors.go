package finance

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgUniqueViolation is the SQLSTATE Postgres returns for a unique-index
// collision (e.g. two concurrent inserts racing for the same number).
const pgUniqueViolation = "23505"

// isUniqueViolation reports whether err wraps a Postgres unique violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

// writeInternalError logs the real cause server-side and returns a generic
// 500 body. Raw database errors carry table, column and constraint names
// that must not reach API clients.
func writeInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "finance: internal error",
		"method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
}

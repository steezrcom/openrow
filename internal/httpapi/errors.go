package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// sqlClientError maps a database error to an HTTP status and a client-safe
// message. The raw error must be logged separately; nothing internal
// (schema, table, constraint, or column names) is allowed in the returned
// message. Returns (0, "") if err is not a recognised SQL error — callers
// should treat that as a validation error and surface err.Error() directly.
func sqlClientError(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return http.StatusNotFound, "not found"
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return http.StatusConflict, "value already exists"
		case "23503":
			return http.StatusConflict, "referenced by other rows"
		case "23502":
			return http.StatusBadRequest, "required field missing"
		case "23514":
			return http.StatusBadRequest, "invalid value"
		case "22P02":
			return http.StatusBadRequest, "invalid input"
		case "42P01":
			return http.StatusNotFound, "not found"
		}
		return http.StatusInternalServerError, "internal error"
	}
	return 0, ""
}

// writeSQLErr logs the raw error and writes a sanitized JSON error. If the
// error isn't a recognised SQL error it's treated as a validation error and
// surfaced verbatim with status 400.
func writeSQLErr(w http.ResponseWriter, log *slog.Logger, err error) {
	status, msg := sqlClientError(err)
	if status == 0 {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if status >= 500 && log != nil {
		log.Error("sql error", slog.Any("err", err), slog.Int("status", status))
	}
	writeErr(w, status, msg)
}

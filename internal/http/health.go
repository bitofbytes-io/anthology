package http

import (
	"context"
	"database/sql/driver"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// readyTimeout bounds the database ping behind /ready so a hung connection
// reports degraded instead of stalling the orchestrator's healthcheck.
const readyTimeout = 2 * time.Second

// Pinger reports whether a dependency is reachable. *sqlx.DB and *sql.DB
// satisfy it.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// healthHandler reports liveness: the process is up and serving requests. It
// never touches dependencies.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

// newReadyHandler reports readiness: 200 when the database answers a ping
// within readyTimeout, 503 otherwise. The response never includes error text.
func newReadyHandler(db Pinger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pingWithTimeout(r.Context(), db); err != nil {
			logger.Warn("readiness check failed", "dependency", "db", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "degraded",
				"db":     "error",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"db":     "ok",
		})
	}
}

// maxReadyPingAttempts caps retries on driver.ErrBadConn. database/sql retries
// bad connections when acquiring one but not during the ping itself, so after
// a database restart each stale pooled connection fails one ping before it is
// discarded. The cap covers the pool's open-connection limit.
const maxReadyPingAttempts = 11

// pingWithTimeout pings db, returning once readyTimeout elapses even if the
// driver does not honour context cancellation.
func pingWithTimeout(ctx context.Context, db Pinger) error {
	if db == nil {
		return errors.New("database not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()

	result := make(chan error, 1)
	go func() {
		err := db.PingContext(ctx)
		for attempt := 1; attempt < maxReadyPingAttempts && errors.Is(err, driver.ErrBadConn) && ctx.Err() == nil; attempt++ {
			err = db.PingContext(ctx)
		}
		result <- err
	}()

	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

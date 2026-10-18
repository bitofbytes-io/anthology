package http

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"anthology/internal/config"
)

func newHealthTestRouter(db Pinger) http.Handler {
	return NewRouter(
		config.Config{Environment: "development"},
		nil,
		nil,
		nil,
		nil,
		nil,
		db,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil,
	)
}

type pingerFunc func(ctx context.Context) error

func (f pingerFunc) PingContext(ctx context.Context) error { return f(ctx) }

func TestHealthRoutes(t *testing.T) {
	// Liveness stays static even when the database is down.
	router := newHealthTestRouter(pingerFunc(func(context.Context) error {
		return errors.New("connection refused")
	}))

	for _, path := range []string{"/health", "/api/health"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)

			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", contentType, "application/json")
			}
			if body := recorder.Body.String(); body != "{\"status\":\"ok\"}\n" {
				t.Fatalf("body = %q, want %q", body, "{\"status\":\"ok\"}\\n")
			}
		})
	}
}

func TestReadyRoutesReportOKWhenDatabasePings(t *testing.T) {
	router := newHealthTestRouter(pingerFunc(func(ctx context.Context) error {
		// The router's 60s request timeout also sets a deadline, so check the
		// ping gets the tighter readiness bound.
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("ping context has no deadline")
		} else if remaining := time.Until(deadline); remaining > readyTimeout {
			t.Errorf("ping deadline in %s, want at most %s", remaining, readyTimeout)
		}
		return nil
	}))

	for _, path := range []string{"/ready", "/api/ready"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			// No session cookie: readiness must stay outside the auth middleware.
			request := httptest.NewRequest(http.MethodGet, path, nil)

			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Fatalf("Content-Type = %q, want %q", contentType, "application/json")
			}
			if body := recorder.Body.String(); body != "{\"db\":\"ok\",\"status\":\"ok\"}\n" {
				t.Fatalf("body = %q, want %q", body, "{\"db\":\"ok\",\"status\":\"ok\"}\\n")
			}
		})
	}
}

func TestReadyRetriesStaleConnections(t *testing.T) {
	// After a database restart every pooled connection fails one ping with
	// driver.ErrBadConn before database/sql discards it.
	stale := 5
	router := newHealthTestRouter(pingerFunc(func(context.Context) error {
		if stale > 0 {
			stale--
			return driver.ErrBadConn
		}
		return nil
	}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestReadyRoutesReportDegradedWhenDatabaseFails(t *testing.T) {
	closedDB, err := sql.Open("postgres", "postgres://anthology:secret-password@127.0.0.1:1/anthology?sslmode=disable")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := closedDB.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	hung := make(chan struct{})
	t.Cleanup(func() { close(hung) })

	cases := []struct {
		name    string
		db      Pinger
		timeout time.Duration
	}{
		{name: "nil database", db: nil},
		{name: "closed database", db: closedDB},
		{name: "failing pinger", db: pingerFunc(func(context.Context) error {
			return errors.New("dial tcp db:5432: password authentication failed for secret-password")
		})},
		{name: "always bad connection", db: pingerFunc(func(context.Context) error {
			return driver.ErrBadConn
		})},
		{
			// A ping that ignores its context still answers once the deadline passes.
			name: "hung pinger",
			db: pingerFunc(func(context.Context) error {
				<-hung
				return nil
			}),
			timeout: 50 * time.Millisecond,
		},
	}

	for _, tc := range cases {
		for _, path := range []string{"/ready", "/api/ready"} {
			t.Run(tc.name+" "+path, func(t *testing.T) {
				router := newHealthTestRouter(tc.db)
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, path, nil)
				if tc.timeout > 0 {
					ctx, cancel := context.WithTimeout(request.Context(), tc.timeout)
					defer cancel()
					request = request.WithContext(ctx)
				}

				router.ServeHTTP(recorder, request)

				if recorder.Code != http.StatusServiceUnavailable {
					t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
				}
				if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
					t.Fatalf("Content-Type = %q, want %q", contentType, "application/json")
				}
				// The exact body also proves no error text or credentials leak.
				if body := recorder.Body.String(); body != "{\"db\":\"error\",\"status\":\"degraded\"}\n" {
					t.Fatalf("body = %q, want %q", body, "{\"db\":\"error\",\"status\":\"degraded\"}\\n")
				}
			})
		}
	}
}

package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"anthology/internal/platform/testdb"
)

func TestNewPostgresReturnsWrappedErrorOnBadDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{"unreachable server", "postgres://127.0.0.1:1/db?sslmode=disable&connect_timeout=1"},
		{"malformed port", "postgres://user:pass@host:notaport/db"},
		{"invalid sslmode", "postgres://127.0.0.1:1/db?sslmode=bogus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			db, err := NewPostgres(ctx, tt.url)
			if err == nil {
				_ = db.Close()
				t.Fatal("expected error")
			}
			if db != nil {
				t.Error("expected nil db on error")
			}
			if !strings.Contains(err.Error(), "connect postgres") {
				t.Errorf("error %q should be wrapped with %q", err, "connect postgres")
			}
		})
	}
}

func TestNewPostgresCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	db, err := NewPostgres(ctx, "postgres://127.0.0.1:1/db?sslmode=disable")
	if err == nil {
		_ = db.Close()
		t.Fatal("expected error for canceled context")
	}
	if db != nil {
		t.Error("expected nil db on error")
	}
}

func TestNewPostgresConfiguresPool(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv(testdb.EnvVar)
	if dsn == "" {
		t.Skip("set " + testdb.EnvVar + " for PostgreSQL regression tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := db.Stats().MaxOpenConnections; got != 10 {
		t.Errorf("MaxOpenConnections = %d, want 10", got)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Errorf("PingContext: %v", err)
	}
	var one int
	if err := db.GetContext(ctx, &one, "SELECT 1"); err != nil || one != 1 {
		t.Errorf("SELECT 1 = %d, err %v", one, err)
	}
}

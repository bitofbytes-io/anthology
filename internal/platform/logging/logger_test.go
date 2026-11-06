package logging

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestNewSelectsLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level string
		want  slog.Level
	}{
		{"debug", "debug", slog.LevelDebug},
		{"info", "info", slog.LevelInfo},
		{"warn", "warn", slog.LevelWarn},
		{"warning alias", "warning", slog.LevelWarn},
		{"error", "error", slog.LevelError},
		{"uppercase", "DEBUG", slog.LevelDebug},
		{"mixed case", "Warning", slog.LevelWarn},
		{"empty defaults to info", "", slog.LevelInfo},
		{"unknown defaults to info", "verbose", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := New(tt.level)
			if logger == nil {
				t.Fatal("expected non-nil logger")
			}

			ctx := context.Background()
			for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
				want := level >= tt.want
				if got := logger.Enabled(ctx, level); got != want {
					t.Errorf("Enabled(%v) = %t, want %t", level, got, want)
				}
			}
		})
	}
}

func TestNewUsesTextHandler(t *testing.T) {
	t.Parallel()

	if got := New("info").Handler(); !isTextHandler(got) {
		t.Fatalf("expected *slog.TextHandler, got %T", got)
	}
}

// TestNewWritesTextToStdout swaps os.Stdout, so it must not run in parallel.
func TestNewWritesTextToStdout(t *testing.T) {
	out := captureStdout(t, func() {
		logger := New("warn")
		logger.Info("dropped message")
		logger.Warn("kept message", "key", "value")
	})

	if strings.Contains(out, "dropped message") {
		t.Errorf("info record should be filtered at warn level, got %q", out)
	}
	for _, want := range []string{"level=WARN", `msg="kept message"`, "key=value"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
}

func isTextHandler(h slog.Handler) bool {
	_, ok := h.(*slog.TextHandler)
	return ok
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan []byte)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.Bytes()
	}()

	fn()

	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out := <-done
	_ = r.Close()
	return string(out)
}

package migrate

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNewGooseLoggerFormatsMigrationSummary(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	adapter, ok := newGooseLogger(logger).(gooseSlogLogger)
	if !ok {
		t.Fatal("expected goose slog logger adapter")
	}

	adapter.Printf("goose: successfully migrated database to version: %d", 7)

	record := decodeLogRecord(t, &buf)
	if got := record["msg"]; got != "goose migrations applied" {
		t.Fatalf("unexpected log message: got %v", got)
	}
	if got := record["component"]; got != "goose" {
		t.Fatalf("unexpected component: got %v", got)
	}
	if got := record["event"]; got != "migration_summary" {
		t.Fatalf("unexpected event: got %v", got)
	}
	if got := record["status"]; got != "applied" {
		t.Fatalf("unexpected status: got %v", got)
	}
	if got := record["version"]; got != float64(7) {
		t.Fatalf("unexpected version: got %v", got)
	}
}

func TestNewGooseLoggerFormatsUpToDateSummaryWithNewline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	adapter, ok := newGooseLogger(logger).(gooseSlogLogger)
	if !ok {
		t.Fatal("expected goose slog logger adapter")
	}

	adapter.Printf("goose: no migrations to run.\ncurrent version: %d", 7)

	record := decodeLogRecord(t, &buf)
	if got := record["msg"]; got != "goose migrations up to date" {
		t.Fatalf("unexpected log message: got %v", got)
	}
	if got := record["event"]; got != "migration_summary" {
		t.Fatalf("unexpected event: got %v", got)
	}
	if got := record["status"]; got != "up_to_date" {
		t.Fatalf("unexpected status: got %v", got)
	}
	if got := record["version"]; got != float64(7) {
		t.Fatalf("unexpected version: got %v", got)
	}
}

func TestNewGooseLoggerFormatsMigrationStep(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	adapter, ok := newGooseLogger(logger).(gooseSlogLogger)
	if !ok {
		t.Fatal("expected goose slog logger adapter")
	}

	adapter.Printf("OK   %s (%s)", "0002_add_shelves.sql", "12.4ms")

	record := decodeLogRecord(t, &buf)
	if got := record["msg"]; got != "goose migration step" {
		t.Fatalf("unexpected log message: got %v", got)
	}
	if got := record["event"]; got != "migration_step" {
		t.Fatalf("unexpected event: got %v", got)
	}
	if got := record["result"]; got != "ok" {
		t.Fatalf("unexpected result: got %v", got)
	}
	if got := record["file"]; got != "0002_add_shelves.sql" {
		t.Fatalf("unexpected file: got %v", got)
	}
	if got := record["duration"]; got != "12.4ms" {
		t.Fatalf("unexpected duration: got %v", got)
	}
}

func TestNewGooseLoggerFormatsMigrationStepWithSingleSpaceAndNewline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	adapter, ok := newGooseLogger(logger).(gooseSlogLogger)
	if !ok {
		t.Fatal("expected goose slog logger adapter")
	}

	adapter.Printf("OK %s (%s)\n", "0002_add_shelves.sql", "12.4ms")

	record := decodeLogRecord(t, &buf)
	if got := record["msg"]; got != "goose migration step" {
		t.Fatalf("unexpected log message: got %v", got)
	}
	if got := record["event"]; got != "migration_step" {
		t.Fatalf("unexpected event: got %v", got)
	}
	if got := record["result"]; got != "ok" {
		t.Fatalf("unexpected result: got %v", got)
	}
	if got := record["file"]; got != "0002_add_shelves.sql" {
		t.Fatalf("unexpected file: got %v", got)
	}
	if got := record["duration"]; got != "12.4ms" {
		t.Fatalf("unexpected duration: got %v", got)
	}
}

func TestGooseSlogLoggerFatalfLogsAndExits(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	exitCode := 0
	adapter := gooseSlogLogger{
		logger: logger.With("component", "goose"),
		exit: func(code int) {
			exitCode = code
		},
	}

	adapter.Fatalf("goose: failed to apply version %d", 5)

	if exitCode != 1 {
		t.Fatalf("unexpected exit code: got %d", exitCode)
	}

	record := decodeLogRecord(t, &buf)
	if got := record["level"]; got != "ERROR" {
		t.Fatalf("unexpected log level: got %v", got)
	}
	if got := record["msg"]; got != "goose log" {
		t.Fatalf("unexpected log message: got %v", got)
	}
	if got := record["detail"]; got != "goose: failed to apply version 5" {
		t.Fatalf("unexpected log detail: got %v", got)
	}
}

func TestNewGooseLoggerNilReturnsSilentLogger(t *testing.T) {
	t.Parallel()

	logger := newGooseLogger(nil)
	if logger == nil {
		t.Fatal("expected non-nil goose logger")
	}

	logger.Printf("should not panic")
	logger.Fatalf("should not exit")
}

func decodeLogRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("failed to decode log record: %v", err)
	}

	return record
}

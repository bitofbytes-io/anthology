package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
)

type gooseSlogLogger struct {
	logger *slog.Logger
	exit   func(int)
}

func newGooseLogger(logger *slog.Logger) goose.Logger {
	if logger == nil {
		return goose.NopLogger()
	}

	return gooseSlogLogger{
		logger: logger.With("component", "goose"),
		exit:   os.Exit,
	}
}

func (l gooseSlogLogger) Printf(format string, v ...interface{}) {
	l.log(slog.LevelInfo, fmt.Sprintf(format, v...))
}

func (l gooseSlogLogger) Fatalf(format string, v ...interface{}) {
	l.log(slog.LevelError, fmt.Sprintf(format, v...))
	if l.exit != nil {
		l.exit(1)
	}
}

func (l gooseSlogLogger) log(level slog.Level, raw string) {
	if l.logger == nil {
		return
	}

	msg, attrs := structuredGooseLog(raw)
	l.logger.Log(context.Background(), level, msg, attrs...)
}

func structuredGooseLog(raw string) (string, []any) {
	if version, ok := parseGooseVersion(raw, "goose: no migrations to run. current version: "); ok {
		return "goose migrations up to date", []any{
			"event", "migration_summary",
			"status", "up_to_date",
			"version", version,
		}
	}

	if version, ok := parseGooseVersion(raw, "goose: successfully migrated database to version: "); ok {
		return "goose migrations applied", []any{
			"event", "migration_summary",
			"status", "applied",
			"version", version,
		}
	}

	if file, duration, ok := parseGooseStep(raw, "OK   "); ok {
		return "goose migration step", []any{
			"event", "migration_step",
			"result", "ok",
			"file", file,
			"duration", duration,
		}
	}

	if file, duration, ok := parseGooseStep(raw, "EMPTY "); ok {
		return "goose migration step", []any{
			"event", "migration_step",
			"result", "empty",
			"file", file,
			"duration", duration,
		}
	}

	return "goose log", []any{
		"event", "migration_log",
		"detail", raw,
	}
}

func parseGooseVersion(raw string, prefix string) (int64, bool) {
	value, ok := strings.CutPrefix(raw, prefix)
	if !ok {
		return 0, false
	}

	version, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, false
	}

	return version, true
}

func parseGooseStep(raw string, prefix string) (string, string, bool) {
	value, ok := strings.CutPrefix(raw, prefix)
	if !ok {
		return "", "", false
	}

	idx := strings.LastIndex(value, " (")
	if idx <= 0 || !strings.HasSuffix(value, ")") {
		return "", "", false
	}

	file := value[:idx]
	duration := strings.TrimSuffix(value[idx+2:], ")")
	if file == "" || duration == "" {
		return "", "", false
	}

	return file, duration, true
}

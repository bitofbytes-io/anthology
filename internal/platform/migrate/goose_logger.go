package migrate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
)

type gooseSlogLogger struct {
	logger *slog.Logger
	exit   func(int)
}

var (
	gooseNoMigrationsPattern = regexp.MustCompile(`^goose:\s+no migrations to run\.\s+current version:\s+(\d+)$`)
	gooseMigratedPattern     = regexp.MustCompile(`^goose:\s+successfully migrated database to version:\s+(\d+)$`)
	gooseStepPattern         = regexp.MustCompile(`^(OK|EMPTY)\s+(.+)\s+\(([^()]+)\)$`)
)

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
	if version, ok := parseGooseVersion(raw, gooseNoMigrationsPattern); ok {
		return "goose migrations up to date", []any{
			"event", "migration_summary",
			"status", "up_to_date",
			"version", version,
		}
	}

	if version, ok := parseGooseVersion(raw, gooseMigratedPattern); ok {
		return "goose migrations applied", []any{
			"event", "migration_summary",
			"status", "applied",
			"version", version,
		}
	}

	if result, file, duration, ok := parseGooseStep(raw); ok {
		return "goose migration step", []any{
			"event", "migration_step",
			"result", result,
			"file", file,
			"duration", duration,
		}
	}

	return "goose log", []any{
		"event", "migration_log",
		"detail", raw,
	}
}

func parseGooseVersion(raw string, pattern *regexp.Regexp) (int64, bool) {
	matches := pattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 2 {
		return 0, false
	}

	version, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, false
	}

	return version, true
}

func parseGooseStep(raw string) (string, string, string, bool) {
	matches := gooseStepPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 4 {
		return "", "", "", false
	}

	result := strings.ToLower(matches[1])
	file := strings.TrimSpace(matches[2])
	duration := strings.TrimSpace(matches[3])
	if file == "" || duration == "" {
		return "", "", "", false
	}

	return result, file, duration, true
}

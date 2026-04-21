package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"

	"anthology/migrations"
)

const (
	baselineVersion int64 = 1
)

// Apply runs any pending SQL migrations bundled with the binary.
func Apply(ctx context.Context, db *sqlx.DB, logger *slog.Logger) error {
	goose.SetBaseFS(migrations.Files)
	goose.SetLogger(newGooseLogger(logger))
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migrate: set goose dialect: %w", err)
	}

	if err := bootstrapBaseline(ctx, db.DB, logger); err != nil {
		return err
	}

	if err := goose.UpContext(ctx, db.DB, "."); err != nil {
		return fmt.Errorf("migrate: goose up: %w", err)
	}

	return nil
}

func bootstrapBaseline(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	coreExists, err := tableExists(ctx, db, "items")
	if err != nil {
		return fmt.Errorf("migrate: check core tables: %w", err)
	}

	if !coreExists {
		return nil
	}

	if _, err := goose.EnsureDBVersionContext(ctx, db); err != nil {
		return fmt.Errorf("migrate: ensure goose table: %w", err)
	}

	current, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("migrate: check goose version: %w", err)
	}

	if current == 0 {
		if err := insertVersion(ctx, db, baselineVersion); err != nil {
			return fmt.Errorf("migrate: set baseline: %w", err)
		}
		if logger != nil {
			logger.Info("goose baseline recorded", "version", baselineVersion)
		}
	}

	return nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	schema, table := splitTableName(name)
	var exists bool
	if schema != "" {
		if err := db.QueryRowContext(
			ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = $1 AND tablename = $2)`,
			schema,
			table,
		).Scan(&exists); err != nil {
			return false, err
		}
		return exists, nil
	}

	if err := db.QueryRowContext(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_tables WHERE (current_schema() IS NULL OR schemaname = current_schema()) AND tablename = $1)`,
		table,
	).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func splitTableName(name string) (string, string) {
	schema, table, found := strings.Cut(name, ".")
	if !found {
		return "", name
	}
	return schema, table
}

func insertVersion(ctx context.Context, db *sql.DB, version int64) error {
	query := fmt.Sprintf(`INSERT INTO %s (version_id, is_applied) VALUES ($1, TRUE)`, goose.TableName())
	_, err := db.ExecContext(ctx, query, version)
	return err
}

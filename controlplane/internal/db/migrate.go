package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.up.sql
var embedMigrations embed.FS

func Migrate(ctx context.Context, databaseURL string) error {
	return migrate(ctx, databaseURL, 0)
}

func MigrateTo(ctx context.Context, databaseURL string, version int64) error {
	if version <= 0 {
		return fmt.Errorf("version must be positive")
	}
	return migrate(ctx, databaseURL, version)
}

func migrate(ctx context.Context, databaseURL string, version int64) error {
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	gdb, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open db for migrate: %w", err)
	}
	defer gdb.Close()

	if err := gdb.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	migrationsFS, err := fs.Sub(embedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("migration fs: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, gdb, migrationsFS)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	defer provider.Close()

	if version == 0 {
		if _, err := provider.Up(ctx); err != nil {
			return fmt.Errorf("goose up: %w", err)
		}
		return nil
	}
	if _, err := provider.UpTo(ctx, version); err != nil {
		return fmt.Errorf("goose up to %d: %w", version, err)
	}
	return nil
}

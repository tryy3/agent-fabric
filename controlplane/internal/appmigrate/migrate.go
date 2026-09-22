package appmigrate

import (
	"context"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
)

func RunMigrations(ctx context.Context, databaseURL, identityPrefix string, deprecated catalog.DeprecatedSandbox) error {
	if err := db.MigrateTo(ctx, databaseURL, 10); err != nil {
		return err
	}
	pool, err := db.OpenPool(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := catalog.Open(pool)
	if _, err := store.EnsurePlaneSettings(ctx, deprecated); err != nil {
		return err
	}
	if err := catalog.BackfillResources(ctx, pool, identityPrefix); err != nil {
		return err
	}
	return db.Migrate(ctx, databaseURL)
}

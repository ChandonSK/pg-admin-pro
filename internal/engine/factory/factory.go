package factory

import (
	"context"
	"fmt"

	"pg-admin-pro/internal/engine"
	"pg-admin-pro/internal/engine/mysql"
	"pg-admin-pro/internal/engine/postgres"
	"pg-admin-pro/internal/engine/sqlite"
)

// Connect instantiates and verifies a database driver for the requested engine.
func Connect(ctx context.Context, cfg engine.ConnectionConfig) (engine.Driver, error) {
	switch cfg.Engine {
	case "", engine.EnginePostgres:
		cfg.Engine = engine.EnginePostgres
		return postgres.New(ctx, cfg)
	case engine.EngineMySQL:
		return mysql.New(ctx, cfg)
	case engine.EngineSQLite:
		return sqlite.New(ctx, cfg)
	default:
		return nil, fmt.Errorf("%w: unsupported engine %q", engine.ErrInvalidInput, cfg.Engine)
	}
}

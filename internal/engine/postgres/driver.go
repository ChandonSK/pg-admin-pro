package postgres

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pg-admin-pro/internal/engine"
)

const maxCachedPools = 16

// Driver implements engine.Driver for PostgreSQL using pgxpool.
type Driver struct {
	cfg      engine.ConnectionConfig
	mu       sync.Mutex
	pools    map[string]*pgxpool.Pool
	lastUsed map[string]time.Time
}

// New connects to the PostgreSQL server and validates credentials with a ping.
func New(ctx context.Context, cfg engine.ConnectionConfig) (*Driver, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	if cfg.Database == "" {
		cfg.Database = "postgres"
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}

	d := &Driver{
		cfg:      cfg,
		pools:    make(map[string]*pgxpool.Pool),
		lastUsed: make(map[string]time.Time),
	}

	if err := d.Ping(ctx); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

func (d *Driver) Kind() engine.EngineKind {
	return engine.EnginePostgres
}

func (d *Driver) Capabilities() engine.Capabilities {
	return engine.Capabilities{
		MultiDatabase:   true,
		UserManagement:  true,
		Schemas:         true,
		BinaryBackup:    true,
		RoleAssignment:  true,
		ActivityMonitor: true,
		ObjectsBrowser:  true,
		ExplainPlan:     true,
	}
}

func (d *Driver) Config() engine.PublicConnectionInfo {
	return engine.PublicConnectionInfo{
		Engine:   engine.EnginePostgres,
		Host:     d.cfg.Host,
		Port:     d.cfg.Port,
		Database: d.cfg.Database,
		Username: d.cfg.Username,
		SSLMode:  d.cfg.SSLMode,
	}
}

// RawConfig returns full credentials internally for pg_dump/pg_restore execution.
func (d *Driver) RawConfig() engine.ConnectionConfig {
	return d.cfg
}

func (d *Driver) buildDSN(dbName string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(d.cfg.Username, d.cfg.Password),
		Host:   fmt.Sprintf("%s:%d", d.cfg.Host, d.cfg.Port),
		Path:   "/" + dbName,
	}
	q := u.Query()
	q.Set("sslmode", d.cfg.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func (d *Driver) getPool(ctx context.Context, dbName string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dbName) == "" {
		dbName = d.cfg.Database
	}

	d.mu.Lock()
	if pool, ok := d.pools[dbName]; ok {
		d.lastUsed[dbName] = time.Now()
		d.mu.Unlock()
		return pool, nil
	}
	d.mu.Unlock()

	poolCfg, err := pgxpool.ParseConfig(d.buildDSN(dbName))
	if err != nil {
		return nil, err
	}
	poolCfg.MaxConns = 5
	poolCfg.MinConns = 0
	poolCfg.MaxConnIdleTime = 2 * time.Minute
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 45 * time.Second
	poolCfg.ConnConfig.ConnectTimeout = 8 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if existing, ok := d.pools[dbName]; ok {
		pool.Close()
		d.lastUsed[dbName] = time.Now()
		return existing, nil
	}

	// Evict least-recently-used non-primary pool if cache exceeds cap to keep RAM minimal.
	if len(d.pools) >= maxCachedPools {
		var oldestKey string
		var oldestTime time.Time
		for k, t := range d.lastUsed {
			if k == d.cfg.Database {
				continue
			}
			if oldestKey == "" || t.Before(oldestTime) {
				oldestKey = k
				oldestTime = t
			}
		}
		if oldestKey != "" {
			d.pools[oldestKey].Close()
			delete(d.pools, oldestKey)
			delete(d.lastUsed, oldestKey)
		}
	}

	d.pools[dbName] = pool
	d.lastUsed[dbName] = time.Now()
	return pool, nil
}

func (d *Driver) dropCachedPool(dbName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p, ok := d.pools[dbName]; ok {
		p.Close()
		delete(d.pools, dbName)
		delete(d.lastUsed, dbName)
	}
}

func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, p := range d.pools {
		p.Close()
		delete(d.pools, k)
	}
	return nil
}

func (d *Driver) Ping(ctx context.Context) error {
	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}
	return pool.Ping(ctx)
}

func (d *Driver) Overview(ctx context.Context) (*engine.ServerOverview, error) {
	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return nil, err
	}

	overview := &engine.ServerOverview{
		Engine:       engine.EnginePostgres,
		Capabilities: d.Capabilities(),
	}

	var uptimeFloat float64
	err = pool.QueryRow(ctx, `
		SELECT
			current_setting('server_version'),
			current_user,
			current_database(),
			(SELECT rolsuper FROM pg_roles WHERE rolname = current_user),
			(SELECT count(*) FROM pg_database WHERE NOT datistemplate),
			(SELECT count(*) FROM pg_roles WHERE rolname NOT LIKE 'pg_%'),
			(SELECT count(*) FROM pg_stat_activity),
			COALESCE((
				SELECT sum(
					CASE WHEN has_database_privilege(current_user, datname, 'CONNECT')
					THEN pg_database_size(datname) ELSE 0 END
				)::bigint
				FROM pg_database WHERE NOT datistemplate
			), 0),
			COALESCE(EXTRACT(EPOCH FROM (now() - pg_postmaster_start_time())), 0)
	`).Scan(
		&overview.Version,
		&overview.CurrentUser,
		&overview.CurrentDatabase,
		&overview.IsAdmin,
		&overview.TotalDatabases,
		&overview.TotalUsers,
		&overview.ActiveConns,
		&overview.TotalSizeBytes,
		&uptimeFloat,
	)
	if err != nil {
		return nil, fmt.Errorf("overview query failed: %w", err)
	}
	overview.UptimeSeconds = int64(uptimeFloat)
	overview.TotalSizePretty = engine.HumanBytes(overview.TotalSizeBytes)

	// Extract concise version number (e.g. "16.4 (Debian...)" -> keep full string or clean)
	if parts := strings.Fields(overview.Version); len(parts) > 0 {
		if _, err := strconv.ParseFloat(parts[0], 64); err == nil {
			overview.Version = "PostgreSQL " + overview.Version
		}
	}

	return overview, nil
}

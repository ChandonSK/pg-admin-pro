package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pg-admin-pro/internal/engine"
)

func (d *Driver) GetActivityOverview(ctx context.Context, dbName string) (*engine.ActivityOverview, error) {
	targetDB := strings.TrimSpace(dbName)
	poolDB := targetDB
	if poolDB == "" {
		poolDB = d.cfg.Database
	}
	pool, err := d.getPool(ctx, poolDB)
	if err != nil {
		return nil, err
	}

	out := &engine.ActivityOverview{
		Sessions:      []engine.ActivitySession{},
		Locks:         []engine.LockInfo{},
		TableStats:    []engine.TablePerfInfo{},
		UnusedIndexes: []engine.UnusedIndexInfo{},
	}

	// Metrics & connection summary
	var hitSum, readSum, tempBytes int64
	err = pool.QueryRow(ctx, `
		SELECT
			COALESCE(sum(blks_hit), 0)::bigint,
			COALESCE(sum(blks_read), 0)::bigint,
			COALESCE(sum(xact_commit), 0)::bigint,
			COALESCE(sum(xact_rollback), 0)::bigint,
			COALESCE(sum(deadlocks), 0)::bigint,
			COALESCE(sum(temp_bytes), 0)::bigint,
			COALESCE(current_setting('max_connections')::int, 100),
			(SELECT count(*)::int FROM pg_stat_activity WHERE state = 'active' AND ($1 = '' OR datname = $1)),
			(SELECT count(*)::int FROM pg_stat_activity WHERE state = 'idle' AND ($1 = '' OR datname = $1)),
			(SELECT count(*)::int FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND ($1 = '' OR datname = $1))
		FROM pg_stat_database
		WHERE ($1 = '' OR datname = $1)
	`, targetDB).Scan(
		&hitSum,
		&readSum,
		&out.Metrics.Commits,
		&out.Metrics.Rollbacks,
		&out.Metrics.Deadlocks,
		&tempBytes,
		&out.Metrics.MaxConnections,
		&out.Metrics.ActiveCount,
		&out.Metrics.IdleCount,
		&out.Metrics.WaitingCount,
	)
	if err == nil {
		out.Metrics.TempBytesPretty = engine.HumanBytes(tempBytes)
		if hitSum+readSum > 0 {
			out.Metrics.CacheHitRatio = float64(int64((float64(hitSum)/float64(hitSum+readSum))*10000)) / 100
		} else {
			out.Metrics.CacheHitRatio = 100
		}
	}

	// Active & Idle Sessions
	sRows, err := pool.Query(ctx, `
		SELECT
			pid,
			COALESCE(datname, ''),
			COALESCE(usename, ''),
			COALESCE(application_name, ''),
			COALESCE(host(client_addr), 'local'),
			COALESCE(state, 'background'),
			COALESCE(NULLIF(CONCAT_WS(':', wait_event_type, wait_event), ''), ''),
			GREATEST(0, COALESCE(EXTRACT(EPOCH FROM (now() - query_start)) * 1000, 0))::bigint,
			COALESCE(to_char(query_start AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE(query, ''),
			pg_blocking_pids(pid)::int[]
		FROM pg_stat_activity
		WHERE pid <> pg_backend_pid()
		  AND ($1 = '' OR datname = $1)
		ORDER BY
			CASE WHEN state = 'active' THEN 0 WHEN state LIKE 'idle in transaction%' THEN 1 ELSE 2 END,
			query_start ASC NULLS LAST
		LIMIT 200
	`, targetDB)
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var s engine.ActivitySession
			var blockers []int32
			if scanErr := sRows.Scan(
				&s.PID, &s.Database, &s.Username, &s.AppName, &s.ClientAddr,
				&s.State, &s.WaitEvent, &s.DurationMs, &s.QueryStart, &s.Query, &blockers,
			); scanErr == nil {
				for _, b := range blockers {
					s.BlockingPIDs = append(s.BlockingPIDs, int(b))
				}
				out.Sessions = append(out.Sessions, s)
			}
		}
	}

	// Locks
	lRows, err := pool.Query(ctx, `
		SELECT
			l.pid,
			COALESCE(d.datname, ''),
			COALESCE(
				CASE WHEN n.nspname IS NOT NULL AND c.relname IS NOT NULL
				     THEN n.nspname || '.' || c.relname
				     ELSE l.relation::text
				END,
				l.locktype
			),
			l.locktype,
			l.mode,
			l.granted,
			COALESCE(a.state, ''),
			COALESCE(a.query, '')
		FROM pg_locks l
		JOIN pg_stat_activity a ON a.pid = l.pid
		LEFT JOIN pg_database d ON d.oid = l.database
		LEFT JOIN pg_class c ON c.oid = l.relation
		LEFT JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE l.pid <> pg_backend_pid()
		  AND ($1 = '' OR d.datname = $1)
		  AND l.locktype IN ('relation', 'tuple', 'transactionid', 'virtualxid', 'object', 'advisory')
		ORDER BY l.granted ASC, a.query_start ASC NULLS LAST
		LIMIT 200
	`, targetDB)
	if err == nil {
		defer lRows.Close()
		for lRows.Next() {
			var l engine.LockInfo
			if scanErr := lRows.Scan(
				&l.PID, &l.Database, &l.Relation, &l.LockType, &l.Mode, &l.Granted, &l.State, &l.Query,
			); scanErr == nil {
				out.Locks = append(out.Locks, l)
			}
		}
	}

	// Per-table performance, disk footprint & dead tuples
	tRows, err := pool.Query(ctx, `
		SELECT
			st.schemaname,
			st.relname,
			pg_total_relation_size(st.relid)::bigint AS total_bytes,
			pg_table_size(st.relid)::bigint AS table_bytes,
			pg_indexes_size(st.relid)::bigint AS index_bytes,
			COALESCE(st.n_live_tup, 0)::bigint,
			COALESCE(st.n_dead_tup, 0)::bigint,
			COALESCE(st.seq_scan, 0)::bigint,
			COALESCE(st.idx_scan, 0)::bigint,
			COALESCE(to_char(GREATEST(st.last_vacuum, st.last_autovacuum) AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI'), ''),
			COALESCE(to_char(GREATEST(st.last_analyze, st.last_autoanalyze) AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI'), '')
		FROM pg_stat_user_tables st
		ORDER BY pg_total_relation_size(st.relid) DESC
		LIMIT 200
	`)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var tp engine.TablePerfInfo
			var tableBytes, indexBytes int64
			if scanErr := tRows.Scan(
				&tp.Schema, &tp.Table, &tp.TotalSizeBytes, &tableBytes, &indexBytes,
				&tp.LiveTuples, &tp.DeadTuples, &tp.SeqScans, &tp.IdxScans,
				&tp.LastVacuum, &tp.LastAnalyze,
			); scanErr == nil {
				tp.TotalSizePretty = engine.HumanBytes(tp.TotalSizeBytes)
				tp.TableSizePretty = engine.HumanBytes(tableBytes)
				tp.IndexSizePretty = engine.HumanBytes(indexBytes)
				out.TableStats = append(out.TableStats, tp)
			}
		}
	}

	// Unused non-unique / non-PK indexes
	uRows, err := pool.Query(ctx, `
		SELECT
			ui.schemaname,
			ui.relname,
			ui.indexrelname,
			ui.idx_scan::bigint,
			pg_relation_size(ui.indexrelid)::bigint AS size_bytes
		FROM pg_stat_user_indexes ui
		JOIN pg_index i ON i.indexrelid = ui.indexrelid
		WHERE ui.idx_scan = 0
		  AND NOT i.indisprimary
		  AND NOT i.indisunique
		ORDER BY pg_relation_size(ui.indexrelid) DESC
		LIMIT 100
	`)
	if err == nil {
		defer uRows.Close()
		for uRows.Next() {
			var ui engine.UnusedIndexInfo
			if scanErr := uRows.Scan(&ui.Schema, &ui.Table, &ui.IndexName, &ui.Scans, &ui.SizeBytes); scanErr == nil {
				ui.SizePretty = engine.HumanBytes(ui.SizeBytes)
				out.UnusedIndexes = append(out.UnusedIndexes, ui)
			}
		}
	}

	return out, nil
}

func (d *Driver) CancelBackend(ctx context.Context, pid int, terminate bool) error {
	if pid <= 0 {
		return fmt.Errorf("%w: invalid backend PID", engine.ErrInvalidInput)
	}
	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}

	fn := "pg_cancel_backend"
	if terminate {
		fn = "pg_terminate_backend"
	}
	var ok bool
	err = pool.QueryRow(ctx, fmt.Sprintf("SELECT %s($1) WHERE $1 <> pg_backend_pid()", fn), pid).Scan(&ok)
	if err != nil || !ok {
		return fmt.Errorf("could not %s pid %d (permission denied or session already ended)", fn, pid)
	}
	return nil
}

func (d *Driver) MaintainTable(ctx context.Context, dbName, schema, table, action string) (*engine.SQLResult, error) {
	schema, table, err := resolveSchemaTable(schema, table)
	if err != nil {
		return nil, err
	}
	qTarget := quoteIdent(schema, table)
	var sqlText, tag string
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "vacuum":
		sqlText, tag = "VACUUM "+qTarget, "VACUUM"
	case "vacuum_analyze":
		sqlText, tag = "VACUUM (ANALYZE) "+qTarget, "VACUUM ANALYZE"
	case "analyze":
		sqlText, tag = "ANALYZE "+qTarget, "ANALYZE"
	case "reindex":
		sqlText, tag = "REINDEX TABLE "+qTarget, "REINDEX TABLE"
	case "refresh_matview":
		sqlText, tag = "REFRESH MATERIALIZED VIEW "+qTarget, "REFRESH MATERIALIZED VIEW"
	default:
		return nil, fmt.Errorf("%w: unsupported maintenance action %q", engine.ErrInvalidInput, action)
	}

	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	if _, err := pool.Exec(ctx, sqlText); err != nil {
		return nil, fmt.Errorf("%s failed: %w", tag, err)
	}
	return &engine.SQLResult{
		Columns:    []string{},
		Rows:       [][]any{},
		DurationMs: time.Since(started).Milliseconds(),
		CommandTag: tag,
	}, nil
}

func (d *Driver) ExplainSQL(ctx context.Context, dbName, sqlText string, analyze bool) (*engine.ExplainResult, error) {
	clean := strings.TrimSpace(sqlText)
	clean = strings.TrimRight(clean, "; \t\r\n")
	if clean == "" {
		return nil, fmt.Errorf("%w: SQL query cannot be empty", engine.ErrInvalidInput)
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	opts := "FORMAT JSON, VERBOSE"
	if analyze {
		opts = "ANALYZE, BUFFERS, FORMAT JSON, VERBOSE"
	}
	explainQuery := fmt.Sprintf("EXPLAIN (%s) %s", opts, clean)

	var rawJSON string
	if err := tx.QueryRow(ctx, explainQuery).Scan(&rawJSON); err != nil {
		return nil, err
	}

	var parsed any
	if err := json.Unmarshal([]byte(rawJSON), &parsed); err != nil {
		return &engine.ExplainResult{
			PlanLines:  []string{rawJSON},
			DurationMs: time.Since(started).Milliseconds(),
			Analyzed:   analyze,
			Engine:     engine.EnginePostgres,
		}, nil
	}

	return &engine.ExplainResult{
		PlanJSON:   parsed,
		DurationMs: time.Since(started).Milliseconds(),
		Analyzed:   analyze,
		Engine:     engine.EnginePostgres,
	}, nil
}

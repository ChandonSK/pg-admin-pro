package postgres

import (
	"context"
	"fmt"
	"strings"

	"pg-admin-pro/internal/engine"
)

func (d *Driver) ListDatabaseObjects(ctx context.Context, dbName, schema string) (*engine.DatabaseObjects, error) {
	s := strings.TrimSpace(schema)
	if s == "" {
		s = "public"
	}
	if _, err := validateName(s); err != nil {
		return nil, fmt.Errorf("%w: invalid schema", engine.ErrInvalidInput)
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	objs := &engine.DatabaseObjects{
		Functions:  []engine.DBFunctionInfo{},
		Triggers:   []engine.DBTriggerInfo{},
		Sequences:  []engine.DBSequenceInfo{},
		Types:      []engine.DBTypeInfo{},
		Extensions: []engine.DBExtensionInfo{},
	}

	// Functions & Procedures
	fRows, err := pool.Query(ctx, `
		SELECT
			n.nspname,
			p.proname,
			pg_get_function_identity_arguments(p.oid) AS args,
			COALESCE(pg_get_function_result(p.oid), 'void') AS ret_type,
			l.lanname,
			CASE p.prokind WHEN 'p' THEN 'procedure' ELSE 'function' END AS kind,
			CASE WHEN l.lanname IN ('sql', 'plpgsql') THEN COALESCE(pg_get_functiondef(p.oid), '') ELSE '' END AS definition
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		JOIN pg_language l ON l.oid = p.prolang
		WHERE n.nspname = $1 AND p.prokind IN ('f', 'p')
		ORDER BY p.proname, args
	`, s)
	if err == nil {
		defer fRows.Close()
		for fRows.Next() {
			var fn engine.DBFunctionInfo
			if scanErr := fRows.Scan(&fn.Schema, &fn.Name, &fn.Args, &fn.ReturnType, &fn.Language, &fn.Kind, &fn.Definition); scanErr == nil {
				objs.Functions = append(objs.Functions, fn)
			}
		}
	}

	// Triggers
	trRows, err := pool.Query(ctx, `
		SELECT
			n.nspname,
			t.tgname,
			c.relname,
			TRIM(BOTH ' ' FROM
				(CASE WHEN (t.tgtype & 2) <> 0 THEN 'BEFORE' WHEN (t.tgtype & 64) <> 0 THEN 'INSTEAD OF' ELSE 'AFTER' END) || ' ' ||
				ARRAY_TO_STRING(ARRAY_REMOVE(ARRAY[
					CASE WHEN (t.tgtype & 4) <> 0 THEN 'INSERT' ELSE NULL END,
					CASE WHEN (t.tgtype & 8) <> 0 THEN 'DELETE' ELSE NULL END,
					CASE WHEN (t.tgtype & 16) <> 0 THEN 'UPDATE' ELSE NULL END,
					CASE WHEN (t.tgtype & 32) <> 0 THEN 'TRUNCATE' ELSE NULL END
				], NULL), ' OR ')
			) AS events,
			(t.tgenabled <> 'D') AS enabled,
			pg_get_triggerdef(t.oid, true) AS definition
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT t.tgisinternal AND n.nspname = $1
		ORDER BY c.relname, t.tgname
	`, s)
	if err == nil {
		defer trRows.Close()
		for trRows.Next() {
			var tr engine.DBTriggerInfo
			if scanErr := trRows.Scan(&tr.Schema, &tr.Name, &tr.Table, &tr.Event, &tr.Enabled, &tr.Definition); scanErr == nil {
				objs.Triggers = append(objs.Triggers, tr)
			}
		}
	}

	// Sequences
	seqRows, err := pool.Query(ctx, `
		SELECT
			schemaname,
			sequencename,
			data_type::text,
			last_value,
			increment_by,
			min_value,
			max_value
		FROM pg_sequences
		WHERE schemaname = $1
		ORDER BY sequencename
	`, s)
	if err == nil {
		defer seqRows.Close()
		for seqRows.Next() {
			var sq engine.DBSequenceInfo
			if scanErr := seqRows.Scan(&sq.Schema, &sq.Name, &sq.DataType, &sq.LastValue, &sq.IncrementBy, &sq.MinValue, &sq.MaxValue); scanErr == nil {
				objs.Sequences = append(objs.Sequences, sq)
			}
		}
	}

	// Custom Types (Enums, Domains, Standalone Composites)
	tyRows, err := pool.Query(ctx, `
		SELECT
			n.nspname,
			t.typname,
			CASE t.typtype WHEN 'e' THEN 'enum' WHEN 'd' THEN 'domain' WHEN 'c' THEN 'composite' ELSE t.typtype::text END AS kind,
			COALESCE((
				SELECT array_agg(e.enumlabel ORDER BY e.enumsortorder)
				FROM pg_enum e
				WHERE e.enumtypid = t.oid
			), '{}') AS values,
			COALESCE(pg_catalog.format_type(NULLIF(t.typbasetype, 0), t.typtypmod), '') AS base_def
		FROM pg_type t
		JOIN pg_namespace n ON n.oid = t.typnamespace
		WHERE n.nspname = $1
		  AND (
			t.typtype IN ('e', 'd')
			OR (t.typtype = 'c' AND EXISTS (SELECT 1 FROM pg_class c WHERE c.oid = t.typrelid AND c.relkind = 'c'))
		  )
		ORDER BY t.typtype, t.typname
	`, s)
	if err == nil {
		defer tyRows.Close()
		for tyRows.Next() {
			var ty engine.DBTypeInfo
			if scanErr := tyRows.Scan(&ty.Schema, &ty.Name, &ty.Kind, &ty.Values, &ty.Definition); scanErr == nil {
				objs.Types = append(objs.Types, ty)
			}
		}
	}

	// Extensions (Database-wide)
	exRows, err := pool.Query(ctx, `
		SELECT
			a.name,
			COALESCE(e.extversion, '') AS installed_version,
			COALESCE(a.default_version, '') AS default_version,
			COALESCE(n.nspname, '') AS schema_name,
			COALESCE(a.comment, '') AS comment,
			(e.oid IS NOT NULL) AS installed
		FROM pg_available_extensions a
		LEFT JOIN pg_extension e ON e.extname = a.name
		LEFT JOIN pg_namespace n ON n.oid = e.extnamespace
		ORDER BY (e.oid IS NOT NULL) DESC, a.name
	`)
	if err == nil {
		defer exRows.Close()
		for exRows.Next() {
			var ex engine.DBExtensionInfo
			if scanErr := exRows.Scan(&ex.Name, &ex.InstalledVersion, &ex.DefaultVersion, &ex.Schema, &ex.Comment, &ex.Installed); scanErr == nil {
				objs.Extensions = append(objs.Extensions, ex)
			}
		}
	}

	return objs, nil
}

func (d *Driver) ManageExtension(ctx context.Context, dbName, name, schema, action string) error {
	extName, err := engine.ValidateName(name)
	if err != nil {
		return err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(action)) {
	case "install":
		sql := "CREATE EXTENSION IF NOT EXISTS " + quoteIdent(extName)
		if s := strings.TrimSpace(schema); s != "" {
			validSchema, err := engine.ValidateName(s)
			if err != nil {
				return err
			}
			sql += " SCHEMA " + quoteIdent(validSchema)
		}
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("install extension: %w", err)
		}
		return nil
	case "drop":
		if _, err := pool.Exec(ctx, "DROP EXTENSION IF EXISTS "+quoteIdent(extName)); err != nil {
			return fmt.Errorf("drop extension: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported extension action %q", engine.ErrInvalidInput, action)
	}
}

func (d *Driver) ListAutocompleteCatalog(ctx context.Context, dbName string) ([]engine.AutocompleteTable, error) {
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT
			n.nspname,
			c.relname,
			CASE c.relkind
				WHEN 'v' THEN 'VIEW'
				WHEN 'm' THEN 'MATERIALIZED VIEW'
				ELSE 'TABLE'
			END AS rtype,
			COALESCE(array_agg(a.attname ORDER BY a.attnum) FILTER (WHERE a.attname IS NOT NULL), '{}') AS cols
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		WHERE c.relkind IN ('r', 'p', 'v', 'm')
		  AND n.nspname NOT LIKE 'pg\_%' ESCAPE '\'
		  AND n.nspname <> 'information_schema'
		GROUP BY n.nspname, c.relname, c.relkind
		ORDER BY CASE WHEN n.nspname = 'public' THEN 0 ELSE 1 END, n.nspname, c.relname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]engine.AutocompleteTable, 0)
	for rows.Next() {
		var item engine.AutocompleteTable
		if scanErr := rows.Scan(&item.Schema, &item.Name, &item.Type, &item.Columns); scanErr == nil {
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

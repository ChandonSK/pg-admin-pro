package postgres

import (
	"context"
	"fmt"
	"strings"

	"pg-admin-pro/internal/engine"
)

func (d *Driver) ListSchemas(ctx context.Context, dbName string, includeSystem bool) ([]engine.SchemaInfo, error) {
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT
			n.nspname,
			pg_get_userbyid(n.nspowner) AS owner,
			(
				SELECT count(*)::int
				FROM pg_class c
				WHERE c.relnamespace = n.oid
				  AND c.relkind IN ('r', 'p', 'v', 'm')
			) AS table_count
		FROM pg_namespace n
		WHERE ($1::bool OR (n.nspname NOT LIKE 'pg\_%' ESCAPE '\' AND n.nspname <> 'information_schema'))
		ORDER BY CASE WHEN n.nspname = 'public' THEN 0 ELSE 1 END, n.nspname
	`, includeSystem)
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	defer rows.Close()

	var schemas []engine.SchemaInfo
	for rows.Next() {
		var s engine.SchemaInfo
		if err := rows.Scan(&s.Name, &s.Owner, &s.TableCount); err != nil {
			return nil, err
		}
		schemas = append(schemas, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if schemas == nil {
		schemas = []engine.SchemaInfo{}
	}
	return schemas, nil
}

func (d *Driver) ListTables(ctx context.Context, dbName, schema string) ([]engine.TableInfo, error) {
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT
			n.nspname,
			c.relname,
			CASE c.relkind
				WHEN 'r' THEN 'BASE TABLE'
				WHEN 'p' THEN 'PARTITIONED TABLE'
				WHEN 'v' THEN 'VIEW'
				WHEN 'm' THEN 'MATERIALIZED VIEW'
				ELSE 'OTHER'
			END AS table_type,
			COALESCE(
				NULLIF(s.n_live_tup, 0),
				CASE WHEN c.reltuples >= 0 THEN c.reltuples::bigint ELSE 0 END
			)::bigint AS row_estimate,
			CASE WHEN c.relkind IN ('r', 'p', 'm') THEN pg_total_relation_size(c.oid) ELSE 0 END::bigint AS size_bytes,
			COALESCE(obj_description(c.oid, 'pg_class'), '') AS comment
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
		WHERE n.nspname = $1
		  AND c.relkind IN ('r', 'p', 'v', 'm')
		ORDER BY c.relkind, c.relname
	`, schema)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()

	var tables []engine.TableInfo
	for rows.Next() {
		var t engine.TableInfo
		if err := rows.Scan(
			&t.Schema,
			&t.Name,
			&t.Type,
			&t.RowEstimate,
			&t.SizeBytes,
			&t.Comment,
		); err != nil {
			return nil, err
		}
		t.TotalSize = engine.HumanBytes(t.SizeBytes)
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if tables == nil {
		tables = []engine.TableInfo{}
	}
	return tables, nil
}

func (d *Driver) GetTableStructure(ctx context.Context, dbName, schema, table string) (*engine.TableStructure, error) {
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}
	if _, err := validateName(schema); err != nil {
		return nil, fmt.Errorf("%w: invalid schema", engine.ErrInvalidInput)
	}
	if _, err := validateName(table); err != nil {
		return nil, fmt.Errorf("%w: invalid table", engine.ErrInvalidInput)
	}

	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	var relOID uint32
	var relKind string
	err = pool.QueryRow(ctx, `
		SELECT c.oid, c.relkind::text
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r', 'p', 'v', 'm')
	`, schema, table).Scan(&relOID, &relKind)
	if err != nil {
		return nil, fmt.Errorf("table %s.%s not found: %w", schema, table, err)
	}

	tableType := "BASE TABLE"
	switch relKind {
	case "v":
		tableType = "VIEW"
	case "m":
		tableType = "MATERIALIZED VIEW"
	case "p":
		tableType = "PARTITIONED TABLE"
	}

	// Primary key attnums
	pkMap := make(map[int]bool)
	pkRows, err := pool.Query(ctx, `
		SELECT unnest(i.indkey)::int
		FROM pg_index i
		WHERE i.indrelid = $1 AND i.indisprimary
	`, relOID)
	if err == nil {
		defer pkRows.Close()
		for pkRows.Next() {
			var attnum int
			if scanErr := pkRows.Scan(&attnum); scanErr == nil {
				pkMap[attnum] = true
			}
		}
	}

	// Foreign key references keyed by local attnum
	fkMap := make(map[int]*engine.ForeignKeyRef)
	fkRows, err := pool.Query(ctx, `
		SELECT
			k.attnum::int,
			con.conname,
			rn.nspname,
			rc.relname,
			ra.attname
		FROM pg_constraint con
		CROSS JOIN LATERAL unnest(con.conkey, con.confkey) WITH ORDINALITY AS k(attnum, ref_attnum, ord)
		JOIN pg_class rc ON rc.oid = con.confrelid
		JOIN pg_namespace rn ON rn.oid = rc.relnamespace
		JOIN pg_attribute ra ON ra.attrelid = rc.oid AND ra.attnum = k.ref_attnum
		WHERE con.conrelid = $1 AND con.contype = 'f'
	`, relOID)
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var attnum int
			var ref engine.ForeignKeyRef
			if scanErr := fkRows.Scan(&attnum, &ref.ConstraintName, &ref.RefSchema, &ref.RefTable, &ref.RefColumn); scanErr == nil {
				fkCopy := ref
				fkMap[attnum] = &fkCopy
			}
		}
	}

	// Enum labels keyed by attnum
	enumMap := make(map[int][]string)
	enumRows, err := pool.Query(ctx, `
		SELECT a.attnum::int, array_agg(e.enumlabel ORDER BY e.enumsortorder)
		FROM pg_attribute a
		JOIN pg_type t ON t.oid = a.atttypid
		JOIN pg_enum e ON e.enumtypid = t.oid
		WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped AND t.typtype = 'e'
		GROUP BY a.attnum
	`, relOID)
	if err == nil {
		defer enumRows.Close()
		for enumRows.Next() {
			var attnum int
			var labels []string
			if scanErr := enumRows.Scan(&attnum, &labels); scanErr == nil {
				enumMap[attnum] = labels
			}
		}
	}

	// Columns
	colRows, err := pool.Query(ctx, `
		SELECT
			a.attnum::int,
			a.attname,
			t.typname,
			pg_catalog.format_type(a.atttypid, a.atttypmod) AS format_type,
			NOT a.attnotnull AS is_nullable,
			pg_get_expr(ad.adbin, ad.adrelid) AS default_expr,
			a.atthasdef,
			a.attidentity <> '' AS is_identity,
			a.attgenerated <> '' AS is_generated
		FROM pg_attribute a
		JOIN pg_type t ON t.oid = a.atttypid
		LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum
	`, relOID)
	if err != nil {
		return nil, fmt.Errorf("query table columns: %w", err)
	}
	defer colRows.Close()

	var cols []engine.ColumnInfo
	var primaryKeys []string
	for colRows.Next() {
		var attnum int
		var c engine.ColumnInfo
		if err := colRows.Scan(
			&attnum,
			&c.Name,
			&c.DataType,
			&c.FormatType,
			&c.IsNullable,
			&c.DefaultValue,
			&c.HasDefault,
			&c.IsIdentity,
			&c.IsGenerated,
		); err != nil {
			return nil, err
		}
		c.OrdinalPos = attnum
		c.IsPrimaryKey = pkMap[attnum]
		if c.IsPrimaryKey {
			primaryKeys = append(primaryKeys, c.Name)
		}
		if fk, ok := fkMap[attnum]; ok {
			c.ForeignKey = fk
		}
		if ev, ok := enumMap[attnum]; ok {
			c.EnumValues = ev
		}
		cols = append(cols, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, err
	}

	// Constraints
	conRows, err := pool.Query(ctx, `
		SELECT
			con.conname,
			CASE con.contype
				WHEN 'p' THEN 'PRIMARY KEY'
				WHEN 'f' THEN 'FOREIGN KEY'
				WHEN 'u' THEN 'UNIQUE'
				WHEN 'c' THEN 'CHECK'
				ELSE con.contype::text
			END AS ctype,
			COALESCE((
				SELECT string_agg(a.attname, ', ' ORDER BY k.ord)
				FROM unnest(con.conkey) WITH ORDINALITY AS k(attnum, ord)
				JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.attnum
			), '') AS columns,
			pg_get_constraintdef(con.oid, true) AS definition
		FROM pg_constraint con
		WHERE con.conrelid = $1 AND con.contype IN ('p', 'f', 'u', 'c')
		ORDER BY CASE con.contype WHEN 'p' THEN 0 WHEN 'f' THEN 1 WHEN 'u' THEN 2 ELSE 3 END, con.conname
	`, relOID)
	var constraints []engine.ConstraintInfo
	if err == nil {
		defer conRows.Close()
		for conRows.Next() {
			var con engine.ConstraintInfo
			if scanErr := conRows.Scan(&con.Name, &con.Type, &con.Columns, &con.Definition); scanErr == nil {
				constraints = append(constraints, con)
			}
		}
	}

	// Indexes
	idxRows, err := pool.Query(ctx, `
		SELECT
			ci.relname AS index_name,
			ix.indisunique,
			ix.indisprimary,
			COALESCE(string_agg(a.attname, ', ' ORDER BY ord.n), '') AS columns,
			pg_get_indexdef(ix.indexrelid) AS definition
		FROM pg_index ix
		JOIN pg_class ci ON ci.oid = ix.indexrelid
		LEFT JOIN LATERAL unnest(ix.indkey) WITH ORDINALITY AS ord(attnum, n) ON true
		LEFT JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = ord.attnum AND ord.attnum > 0
		WHERE ix.indrelid = $1
		GROUP BY ci.relname, ix.indisunique, ix.indisprimary, ix.indexrelid
		ORDER BY ix.indisprimary DESC, ci.relname
	`, relOID)
	var indexes []engine.IndexInfo
	if err == nil {
		defer idxRows.Close()
		for idxRows.Next() {
			var idx engine.IndexInfo
			if scanErr := idxRows.Scan(&idx.Name, &idx.IsUnique, &idx.IsPrimary, &idx.Columns, &idx.Definition); scanErr == nil {
				indexes = append(indexes, idx)
			}
		}
	}

	if cols == nil {
		cols = []engine.ColumnInfo{}
	}
	if indexes == nil {
		indexes = []engine.IndexInfo{}
	}
	if constraints == nil {
		constraints = []engine.ConstraintInfo{}
	}
	if primaryKeys == nil {
		primaryKeys = []string{}
	}

	return &engine.TableStructure{
		Schema:            schema,
		Table:             table,
		Type:              tableType,
		Columns:           cols,
		Indexes:           indexes,
		Constraints:       constraints,
		PrimaryKeys:       primaryKeys,
		UsesRowIDFallback: len(primaryKeys) == 0 && (relKind == "r" || relKind == "p"),
	}, nil
}

func (d *Driver) CreateTable(ctx context.Context, dbName string, req engine.CreateTableReq) error {
	schema := strings.TrimSpace(req.Schema)
	if schema == "" {
		schema = "public"
	}
	if _, err := validateName(schema); err != nil {
		return fmt.Errorf("%w: invalid schema", engine.ErrInvalidInput)
	}
	table, err := validateName(req.Table)
	if err != nil {
		return fmt.Errorf("%w: invalid table name", engine.ErrInvalidInput)
	}
	if len(req.Columns) == 0 {
		return fmt.Errorf("%w: table must have at least one column", engine.ErrInvalidInput)
	}

	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}

	var colDefs []string
	var pkCols []string
	for _, col := range req.Columns {
		cname, err := engine.ValidateName(col.Name)
		if err != nil {
			return err
		}
		ctype, err := engine.ValidateColumnType(col.Type)
		if err != nil {
			return err
		}

		var def strings.Builder
		def.WriteString(quoteIdent(cname))
		def.WriteString(" ")
		def.WriteString(ctype)
		if !col.Nullable && !col.PrimaryKey {
			def.WriteString(" NOT NULL")
		}
		if dflt, err := engine.ValidateDefaultExpr(col.Default); err != nil {
			return err
		} else if dflt != "" {
			def.WriteString(" DEFAULT ")
			def.WriteString(dflt)
		}
		colDefs = append(colDefs, def.String())
		if col.PrimaryKey {
			pkCols = append(pkCols, quoteIdent(cname))
		}
	}

	if len(pkCols) > 0 {
		colDefs = append(colDefs, "PRIMARY KEY ("+strings.Join(pkCols, ", ")+")")
	}

	sql := fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdent(schema, table), strings.Join(colDefs, ", "))
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	return nil
}

func (d *Driver) DropTable(ctx context.Context, dbName, schema, table string) error {
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}
	if _, err := validateName(schema); err != nil {
		return fmt.Errorf("%w: invalid schema", engine.ErrInvalidInput)
	}
	if _, err := validateName(table); err != nil {
		return fmt.Errorf("%w: invalid table", engine.ErrInvalidInput)
	}

	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}

	sql := "DROP TABLE " + quoteIdent(schema, table)
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("drop table: %w", err)
	}
	return nil
}

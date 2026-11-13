package postgres

import (
	"context"
	"fmt"
	"strings"

	"pg-admin-pro/internal/engine"
)

func resolveSchemaTable(schema, table string) (string, string, error) {
	s := strings.TrimSpace(schema)
	if s == "" {
		s = "public"
	}
	var err error
	s, err = engine.ValidateName(s)
	if err != nil {
		return "", "", fmt.Errorf("%w: schema: %v", engine.ErrInvalidInput, err)
	}
	t, err := engine.ValidateName(table)
	if err != nil {
		return "", "", fmt.Errorf("%w: table: %v", engine.ErrInvalidInput, err)
	}
	return s, t, nil
}

func (d *Driver) AddColumn(ctx context.Context, dbName string, req engine.AddColumnReq) error {
	schema, table, err := resolveSchemaTable(req.Schema, req.Table)
	if err != nil {
		return err
	}
	colName, err := engine.ValidateName(req.Name)
	if err != nil {
		return err
	}
	colType, err := engine.ValidateColumnType(req.Type)
	if err != nil {
		return err
	}
	dflt, err := engine.ValidateDefaultExpr(req.Default)
	if err != nil {
		return err
	}

	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("ALTER TABLE ")
	b.WriteString(quoteIdent(schema, table))
	b.WriteString(" ADD COLUMN ")
	b.WriteString(quoteIdent(colName))
	b.WriteString(" ")
	b.WriteString(colType)
	if !req.Nullable {
		b.WriteString(" NOT NULL")
	}
	if dflt != "" {
		b.WriteString(" DEFAULT ")
		b.WriteString(dflt)
	}

	if _, err := pool.Exec(ctx, b.String()); err != nil {
		return fmt.Errorf("add column: %w", err)
	}
	return nil
}

func (d *Driver) AlterColumn(ctx context.Context, dbName string, req engine.AlterColumnReq) error {
	schema, table, err := resolveSchemaTable(req.Schema, req.Table)
	if err != nil {
		return err
	}
	colName, err := engine.ValidateName(req.Column)
	if err != nil {
		return err
	}

	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}

	qTable := quoteIdent(schema, table)
	qCol := quoteIdent(colName)
	var stmts []string

	if req.SetNullable != nil {
		if *req.SetNullable {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL", qTable, qCol))
		} else {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL", qTable, qCol))
		}
	}

	if req.DropDefault {
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT", qTable, qCol))
	} else if req.SetDefault != nil {
		dflt, err := engine.ValidateDefaultExpr(*req.SetDefault)
		if err != nil {
			return err
		}
		if dflt == "" {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT", qTable, qCol))
		} else {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s", qTable, qCol, dflt))
		}
	}

	if strings.TrimSpace(req.NewType) != "" {
		colType, err := engine.ValidateColumnType(req.NewType)
		if err != nil {
			return err
		}
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s", qTable, qCol, colType, qCol, colType))
	}

	if newName := strings.TrimSpace(req.NewName); newName != "" && newName != colName {
		validatedNew, err := engine.ValidateName(newName)
		if err != nil {
			return err
		}
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", qTable, qCol, quoteIdent(validatedNew)))
	}

	if len(stmts) == 0 {
		return fmt.Errorf("%w: no column alterations specified", engine.ErrInvalidInput)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("alter column: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (d *Driver) DropColumn(ctx context.Context, dbName, schema, table, column string) error {
	schema, table, err := resolveSchemaTable(schema, table)
	if err != nil {
		return err
	}
	col, err := engine.ValidateName(column)
	if err != nil {
		return err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdent(schema, table), quoteIdent(col))
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("drop column: %w", err)
	}
	return nil
}

func (d *Driver) TruncateTable(ctx context.Context, dbName, schema, table string, cascade bool) error {
	schema, table, err := resolveSchemaTable(schema, table)
	if err != nil {
		return err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}
	sql := "TRUNCATE TABLE " + quoteIdent(schema, table)
	if cascade {
		sql += " CASCADE"
	}
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("truncate table: %w", err)
	}
	return nil
}

func (d *Driver) CreateIndex(ctx context.Context, dbName string, req engine.CreateIndexReq) error {
	schema, table, err := resolveSchemaTable(req.Schema, req.Table)
	if err != nil {
		return err
	}
	if len(req.Columns) == 0 {
		return fmt.Errorf("%w: at least one column is required for an index", engine.ErrInvalidInput)
	}
	qCols := make([]string, 0, len(req.Columns))
	for _, c := range req.Columns {
		valid, err := engine.ValidateName(c)
		if err != nil {
			return err
		}
		qCols = append(qCols, quoteIdent(valid))
	}
	method, err := engine.ValidateIndexMethod(req.Method)
	if err != nil {
		return err
	}

	idxName := strings.TrimSpace(req.Name)
	if idxName == "" {
		idxName = fmt.Sprintf("idx_%s_%s", table, strings.Join(req.Columns, "_"))
		if len(idxName) > 63 {
			idxName = idxName[:63]
		}
	}
	idxName, err = engine.ValidateName(idxName)
	if err != nil {
		return err
	}

	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("CREATE ")
	if req.Unique {
		b.WriteString("UNIQUE ")
	}
	b.WriteString("INDEX ")
	b.WriteString(quoteIdent(idxName))
	b.WriteString(" ON ")
	b.WriteString(quoteIdent(schema, table))
	if method != "" {
		b.WriteString(" USING ")
		b.WriteString(method)
	}
	b.WriteString(" (")
	b.WriteString(strings.Join(qCols, ", "))
	b.WriteString(")")

	if _, err := pool.Exec(ctx, b.String()); err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	return nil
}

func (d *Driver) DropIndex(ctx context.Context, dbName, schema, _, indexName string) error {
	s := strings.TrimSpace(schema)
	if s == "" {
		s = "public"
	}
	var err error
	s, err = engine.ValidateName(s)
	if err != nil {
		return err
	}
	idx, err := engine.ValidateName(indexName)
	if err != nil {
		return err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, "DROP INDEX "+quoteIdent(s, idx)); err != nil {
		return fmt.Errorf("drop index: %w", err)
	}
	return nil
}

func (d *Driver) AddConstraint(ctx context.Context, dbName string, req engine.AddConstraintReq) error {
	schema, table, err := resolveSchemaTable(req.Schema, req.Table)
	if err != nil {
		return err
	}
	ctype := strings.ToUpper(strings.TrimSpace(req.Type))

	var clause string
	cname := strings.TrimSpace(req.Name)
	switch ctype {
	case "UNIQUE":
		if len(req.Columns) == 0 {
			return fmt.Errorf("%w: columns required for UNIQUE constraint", engine.ErrInvalidInput)
		}
		qCols, err := validateQuoteCols(req.Columns)
		if err != nil {
			return err
		}
		if cname == "" {
			cname = fmt.Sprintf("uq_%s_%s", table, strings.Join(req.Columns, "_"))
		}
		clause = fmt.Sprintf("UNIQUE (%s)", strings.Join(qCols, ", "))
	case "FOREIGN KEY":
		if len(req.Columns) == 0 || len(req.Columns) != len(req.RefColumns) {
			return fmt.Errorf("%w: source and referenced columns must match for FOREIGN KEY", engine.ErrInvalidInput)
		}
		qCols, err := validateQuoteCols(req.Columns)
		if err != nil {
			return err
		}
		qRefCols, err := validateQuoteCols(req.RefColumns)
		if err != nil {
			return err
		}
		refSchema, refTable, err := resolveSchemaTable(req.RefSchema, req.RefTable)
		if err != nil {
			return err
		}
		onDel, err := engine.ValidateFKAction(req.OnDelete)
		if err != nil {
			return err
		}
		onUpd, err := engine.ValidateFKAction(req.OnUpdate)
		if err != nil {
			return err
		}
		if cname == "" {
			cname = fmt.Sprintf("fk_%s_%s", table, strings.Join(req.Columns, "_"))
		}
		clause = fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s (%s)", strings.Join(qCols, ", "), quoteIdent(refSchema, refTable), strings.Join(qRefCols, ", "))
		if onDel != "" {
			clause += " ON DELETE " + onDel
		}
		if onUpd != "" {
			clause += " ON UPDATE " + onUpd
		}
	case "CHECK":
		expr, err := engine.ValidateCheckExpr(req.CheckExpr)
		if err != nil {
			return err
		}
		if cname == "" {
			cname = fmt.Sprintf("chk_%s_rule", table)
		}
		clause = fmt.Sprintf("CHECK (%s)", expr)
	default:
		return fmt.Errorf("%w: unsupported constraint type %q", engine.ErrInvalidInput, req.Type)
	}

	if len(cname) > 63 {
		cname = cname[:63]
	}
	cname, err = engine.ValidateName(cname)
	if err != nil {
		return err
	}

	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s", quoteIdent(schema, table), quoteIdent(cname), clause)
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("add constraint: %w", err)
	}
	return nil
}

func (d *Driver) DropConstraint(ctx context.Context, dbName, schema, table, constraintName string) error {
	schema, table, err := resolveSchemaTable(schema, table)
	if err != nil {
		return err
	}
	cname, err := engine.ValidateName(constraintName)
	if err != nil {
		return err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}
	sql := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", quoteIdent(schema, table), quoteIdent(cname))
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("drop constraint: %w", err)
	}
	return nil
}

func validateQuoteCols(cols []string) ([]string, error) {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		v, err := engine.ValidateName(c)
		if err != nil {
			return nil, err
		}
		out = append(out, quoteIdent(v))
	}
	return out, nil
}

package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"pg-admin-pro/internal/engine"
)

func (d *Driver) AddColumn(ctx context.Context, _ string, req engine.AddColumnReq) error {
	tname, err := engine.ValidateName(req.Table)
	if err != nil {
		return err
	}
	cname, err := engine.ValidateName(req.Name)
	if err != nil {
		return err
	}
	ctype, err := engine.ValidateColumnType(req.Type)
	if err != nil {
		return err
	}
	dflt, err := engine.ValidateDefaultExpr(req.Default)
	if err != nil {
		return err
	}
	q := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quoteIdent(tname), quoteIdent(cname), ctype)
	if !req.Nullable {
		q += " NOT NULL"
	}
	if dflt != "" {
		q += " DEFAULT " + dflt
	}
	_, err = d.db.ExecContext(ctx, q)
	return err
}

func (d *Driver) AlterColumn(ctx context.Context, _ string, req engine.AlterColumnReq) error {
	tname, err := engine.ValidateName(req.Table)
	if err != nil {
		return err
	}
	cname, err := engine.ValidateName(req.Column)
	if err != nil {
		return err
	}
	newName := strings.TrimSpace(req.NewName)
	if newName != "" && newName != cname && req.NewType == "" && req.SetNullable == nil && req.SetDefault == nil && !req.DropDefault {
		v, err := engine.ValidateName(newName)
		if err != nil {
			return err
		}
		_, err = d.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", quoteIdent(tname), quoteIdent(cname), quoteIdent(v)))
		return err
	}
	return fmt.Errorf("%w: SQLite only supports RENAME COLUMN via ALTER COLUMN (use table rebuild for type/nullability changes)", engine.ErrUnsupported)
}

func (d *Driver) DropColumn(ctx context.Context, _, _, table, column string) error {
	tname, err := engine.ValidateName(table)
	if err != nil {
		return err
	}
	cname, err := engine.ValidateName(column)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdent(tname), quoteIdent(cname)))
	return err
}

func (d *Driver) TruncateTable(ctx context.Context, _, _, table string, _ bool) error {
	tname, err := engine.ValidateName(table)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, "DELETE FROM "+quoteIdent(tname))
	return err
}

func (d *Driver) CreateIndex(ctx context.Context, _ string, req engine.CreateIndexReq) error {
	tname, err := engine.ValidateName(req.Table)
	if err != nil {
		return err
	}
	if len(req.Columns) == 0 {
		return fmt.Errorf("%w: at least one column required", engine.ErrInvalidInput)
	}
	var qCols []string
	for _, c := range req.Columns {
		vc, err := engine.ValidateName(c)
		if err != nil {
			return err
		}
		qCols = append(qCols, quoteIdent(vc))
	}
	idxName := strings.TrimSpace(req.Name)
	if idxName == "" {
		idxName = fmt.Sprintf("idx_%s_%s", tname, strings.Join(req.Columns, "_"))
	}
	idxName, err = engine.ValidateName(idxName)
	if err != nil {
		return err
	}
	uniq := ""
	if req.Unique {
		uniq = "UNIQUE "
	}
	_, err = d.db.ExecContext(ctx, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", uniq, quoteIdent(idxName), quoteIdent(tname), strings.Join(qCols, ", ")))
	return err
}

func (d *Driver) DropIndex(ctx context.Context, _, _, _, indexName string) error {
	iname, err := engine.ValidateName(indexName)
	if err != nil {
		return err
	}
	_, err = d.db.ExecContext(ctx, "DROP INDEX "+quoteIdent(iname))
	return err
}

func (d *Driver) AddConstraint(context.Context, string, engine.AddConstraintReq) error {
	return fmt.Errorf("%w: SQLite does not support ALTER TABLE ADD CONSTRAINT directly", engine.ErrUnsupported)
}

func (d *Driver) DropConstraint(context.Context, string, string, string, string) error {
	return fmt.Errorf("%w: SQLite does not support ALTER TABLE DROP CONSTRAINT directly", engine.ErrUnsupported)
}

func (d *Driver) StreamRows(
	ctx context.Context,
	dbName string,
	req engine.QueryRowsReq,
	allRows bool,
	onHeader func([]engine.ColumnInfo) error,
	onRow func([]any) error,
) error {
	st, err := d.GetTableStructure(ctx, dbName, "main", req.Table)
	if err != nil {
		return err
	}
	colMap := make(map[string]engine.ColumnInfo, len(st.Columns))
	var selectCols []string
	for _, c := range st.Columns {
		colMap[c.Name] = c
		selectCols = append(selectCols, quoteIdent(c.Name))
	}
	if err := onHeader(st.Columns); err != nil {
		return err
	}

	var whereParts []string
	var args []any
	if s := strings.TrimSpace(req.Search); s != "" {
		var ors []string
		for _, c := range st.Columns {
			ors = append(ors, fmt.Sprintf("CAST(%s AS TEXT) LIKE ?", quoteIdent(c.Name)))
			args = append(args, "%"+s+"%")
		}
		whereParts = append(whereParts, "("+strings.Join(ors, " OR ")+")")
	}
	for _, f := range req.Filters {
		if _, ok := colMap[f.Column]; !ok {
			return fmt.Errorf("%w: unknown column %q", engine.ErrInvalidInput, f.Column)
		}
		qc := quoteIdent(f.Column)
		switch f.Operator {
		case "is_null":
			whereParts = append(whereParts, qc+" IS NULL")
		case "is_not_null":
			whereParts = append(whereParts, qc+" IS NOT NULL")
		case "ilike", "like":
			val := f.Value
			if !strings.ContainsAny(val, "%_") {
				val = "%" + val + "%"
			}
			args = append(args, val)
			whereParts = append(whereParts, fmt.Sprintf("CAST(%s AS TEXT) LIKE ?", qc))
		case "=", "!=", ">", ">=", "<", "<=":
			op := f.Operator
			if op == "!=" {
				op = "<>"
			}
			args = append(args, f.Value)
			whereParts = append(whereParts, fmt.Sprintf("%s %s ?", qc, op))
		}
	}
	whereSQL := ""
	if len(whereParts) > 0 {
		whereSQL = " WHERE " + strings.Join(whereParts, " AND ")
	}
	orderSQL := ""
	if req.SortBy != "" {
		if _, ok := colMap[req.SortBy]; ok {
			dir := "ASC"
			if strings.EqualFold(req.SortDir, "desc") {
				dir = "DESC"
			}
			orderSQL = " ORDER BY " + quoteIdent(req.SortBy) + " " + dir
		}
	}
	sqlText := fmt.Sprintf("SELECT %s FROM %s%s%s", strings.Join(selectCols, ", "), quoteIdent(req.Table), whereSQL, orderSQL)
	if !allRows {
		if req.Page < 1 {
			req.Page = 1
		}
		if req.PageSize < 1 || req.PageSize > 5000 {
			req.PageSize = 50
		}
		offset := (req.Page - 1) * req.PageSize
		sqlText += " LIMIT ? OFFSET ?"
		args = append(args, req.PageSize, offset)
	}

	rows, err := d.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	colCount := len(st.Columns)
	for rows.Next() {
		raw := make([]any, colCount)
		ptrs := make([]any, colCount)
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		for i, v := range raw {
			if b, ok := v.([]byte); ok {
				raw[i] = string(b)
			}
		}
		if err := onRow(raw); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (d *Driver) ImportRows(ctx context.Context, dbName, _, table string, req engine.ImportRowsReq) (*engine.ImportResult, error) {
	if len(req.Rows) == 0 {
		return nil, fmt.Errorf("%w: no rows supplied", engine.ErrInvalidInput)
	}
	started := time.Now()
	st, err := d.GetTableStructure(ctx, dbName, "main", table)
	if err != nil {
		return nil, err
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res := &engine.ImportResult{}
	for idx, row := range req.Rows {
		var cols, phs []string
		var args []any
		for _, c := range st.Columns {
			if v, ok := row[c.Name]; ok {
				cols = append(cols, quoteIdent(c.Name))
				phs = append(phs, "?")
				args = append(args, v)
			}
		}
		q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteIdent(table), strings.Join(cols, ", "), strings.Join(phs, ", "))
		if len(cols) == 0 {
			q = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", quoteIdent(table))
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			if !req.ContinueOnError {
				return nil, fmt.Errorf("row %d: %w", idx+1, err)
			}
			res.Failed++
			if len(res.Errors) < 100 {
				res.Errors = append(res.Errors, engine.ImportRowError{RowIndex: idx + 1, Error: err.Error()})
			}
			continue
		}
		res.Inserted++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	res.DurationMs = time.Since(started).Milliseconds()
	return res, nil
}

func (d *Driver) GetActivityOverview(context.Context, string) (*engine.ActivityOverview, error) {
	return nil, engine.ErrUnsupported
}

func (d *Driver) CancelBackend(context.Context, int, bool) error {
	return engine.ErrUnsupported
}

func (d *Driver) MaintainTable(ctx context.Context, _, _, table, action string) (*engine.SQLResult, error) {
	tname, err := engine.ValidateName(table)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	var q, tag string
	switch strings.ToLower(action) {
	case "analyze", "vacuum_analyze":
		q, tag = "ANALYZE "+quoteIdent(tname), "ANALYZE"
	case "vacuum":
		q, tag = "VACUUM", "VACUUM"
	case "reindex":
		q, tag = "REINDEX "+quoteIdent(tname), "REINDEX"
	default:
		return nil, engine.ErrUnsupported
	}
	if _, err := d.db.ExecContext(ctx, q); err != nil {
		return nil, err
	}
	return &engine.SQLResult{Columns: []string{}, Rows: [][]any{}, DurationMs: time.Since(started).Milliseconds(), CommandTag: tag}, nil
}

func (d *Driver) ExplainSQL(ctx context.Context, _, sqlText string, _ bool) (*engine.ExplainResult, error) {
	clean := strings.TrimRight(strings.TrimSpace(sqlText), "; \t\r\n")
	if clean == "" {
		return nil, fmt.Errorf("%w: SQL query cannot be empty", engine.ErrInvalidInput)
	}
	started := time.Now()
	rows, err := d.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if rows.Scan(&id, &parent, &notused, &detail) == nil {
			lines = append(lines, fmt.Sprintf("[%d <- %d] %s", id, parent, detail))
		}
	}
	return &engine.ExplainResult{PlanLines: lines, DurationMs: time.Since(started).Milliseconds(), Analyzed: false, Engine: engine.EngineSQLite}, rows.Err()
}

func (d *Driver) ListDatabaseObjects(ctx context.Context, _, _ string) (*engine.DatabaseObjects, error) {
	objs := &engine.DatabaseObjects{
		Functions:  []engine.DBFunctionInfo{},
		Triggers:   []engine.DBTriggerInfo{},
		Sequences:  []engine.DBSequenceInfo{},
		Types:      []engine.DBTypeInfo{},
		Extensions: []engine.DBExtensionInfo{},
	}
	rows, err := d.db.QueryContext(ctx, `SELECT name, tbl_name, COALESCE(sql, '') FROM sqlite_master WHERE type = 'trigger' ORDER BY name`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tr engine.DBTriggerInfo
			tr.Schema = "main"
			tr.Enabled = true
			if rows.Scan(&tr.Name, &tr.Table, &tr.Definition) == nil {
				objs.Triggers = append(objs.Triggers, tr)
			}
		}
	}
	return objs, nil
}

func (d *Driver) ManageExtension(context.Context, string, string, string, string) error {
	return engine.ErrUnsupported
}

func (d *Driver) ListAutocompleteCatalog(ctx context.Context, dbName string) ([]engine.AutocompleteTable, error) {
	tables, err := d.ListTables(ctx, dbName, "main")
	if err != nil {
		return nil, err
	}
	out := make([]engine.AutocompleteTable, 0, len(tables))
	for _, t := range tables {
		st, stErr := d.GetTableStructure(ctx, dbName, "main", t.Name)
		cols := []string{}
		if stErr == nil {
			for _, c := range st.Columns {
				cols = append(cols, c.Name)
			}
		}
		out = append(out, engine.AutocompleteTable{
			Schema:  "main",
			Name:    t.Name,
			Type:    t.Type,
			Columns: cols,
		})
	}
	return out, nil
}

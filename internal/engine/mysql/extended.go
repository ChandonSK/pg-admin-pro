package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pg-admin-pro/internal/engine"
)

func parseMySQLEnum(colType string) []string {
	low := strings.ToLower(strings.TrimSpace(colType))
	if !strings.HasPrefix(low, "enum(") || !strings.HasSuffix(colType, ")") {
		return nil
	}
	inner := colType[5 : len(colType)-1]
	var vals []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if ch == '\'' {
			if inQuote && i+1 < len(inner) && inner[i+1] == '\'' {
				cur.WriteByte('\'')
				i++
				continue
			}
			inQuote = !inQuote
			continue
		}
		if ch == ',' && !inQuote {
			vals = append(vals, cur.String())
			cur.Reset()
			continue
		}
		if inQuote {
			cur.WriteByte(ch)
		}
	}
	if cur.Len() > 0 {
		vals = append(vals, cur.String())
	}
	return vals
}

func (d *Driver) AddColumn(ctx context.Context, dbName string, req engine.AddColumnReq) error {
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
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	sqlText := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quoteIdent(tname), quoteIdent(cname), ctype)
	if !req.Nullable {
		sqlText += " NOT NULL"
	}
	if dflt != "" {
		sqlText += " DEFAULT " + dflt
	}
	_, err = db.ExecContext(ctx, sqlText)
	return err
}

func (d *Driver) AlterColumn(ctx context.Context, dbName string, req engine.AlterColumnReq) error {
	st, err := d.GetTableStructure(ctx, dbName, dbName, req.Table)
	if err != nil {
		return err
	}
	var current *engine.ColumnInfo
	for i := range st.Columns {
		if st.Columns[i].Name == req.Column {
			current = &st.Columns[i]
			break
		}
	}
	if current == nil {
		return fmt.Errorf("%w: column %q not found", engine.ErrInvalidInput, req.Column)
	}

	colType := current.FormatType
	if strings.TrimSpace(req.NewType) != "" {
		validated, err := engine.ValidateColumnType(req.NewType)
		if err != nil {
			return err
		}
		colType = validated
	}
	nullable := current.IsNullable
	if req.SetNullable != nil {
		nullable = *req.SetNullable
	}
	newName := current.Name
	if strings.TrimSpace(req.NewName) != "" {
		v, err := engine.ValidateName(req.NewName)
		if err != nil {
			return err
		}
		newName = v
	}

	var defClause string
	if req.DropDefault {
		defClause = ""
	} else if req.SetDefault != nil {
		v, err := engine.ValidateDefaultExpr(*req.SetDefault)
		if err != nil {
			return err
		}
		if v != "" {
			defClause = " DEFAULT " + v
		}
	} else if current.HasDefault && current.DefaultValue != nil {
		if v, err := engine.ValidateDefaultExpr(*current.DefaultValue); err == nil && v != "" {
			defClause = " DEFAULT " + v
		}
	}

	nullClause := "NULL"
	if !nullable {
		nullClause = "NOT NULL"
	}

	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	stmt := fmt.Sprintf(
		"ALTER TABLE %s CHANGE COLUMN %s %s %s %s%s",
		quoteIdent(req.Table), quoteIdent(req.Column), quoteIdent(newName), colType, nullClause, defClause,
	)
	_, err = db.ExecContext(ctx, stmt)
	return err
}

func (d *Driver) DropColumn(ctx context.Context, dbName, _, table, column string) error {
	tname, err := engine.ValidateName(table)
	if err != nil {
		return err
	}
	cname, err := engine.ValidateName(column)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", quoteIdent(tname), quoteIdent(cname)))
	return err
}

func (d *Driver) TruncateTable(ctx context.Context, dbName, _, table string, _ bool) error {
	tname, err := engine.ValidateName(table)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "TRUNCATE TABLE "+quoteIdent(tname))
	return err
}

func (d *Driver) CreateIndex(ctx context.Context, dbName string, req engine.CreateIndexReq) error {
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
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	uniq := ""
	if req.Unique {
		uniq = "UNIQUE "
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s)", uniq, quoteIdent(idxName), quoteIdent(tname), strings.Join(qCols, ", ")))
	return err
}

func (d *Driver) DropIndex(ctx context.Context, dbName, _, table, indexName string) error {
	tname, err := engine.ValidateName(table)
	if err != nil {
		return err
	}
	iname, err := engine.ValidateName(indexName)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP INDEX %s ON %s", quoteIdent(iname), quoteIdent(tname)))
	return err
}

func (d *Driver) AddConstraint(ctx context.Context, dbName string, req engine.AddConstraintReq) error {
	tname, err := engine.ValidateName(req.Table)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	cname := strings.TrimSpace(req.Name)
	ctype := strings.ToUpper(strings.TrimSpace(req.Type))

	var clause string
	switch ctype {
	case "UNIQUE":
		if len(req.Columns) == 0 {
			return fmt.Errorf("%w: columns required", engine.ErrInvalidInput)
		}
		qCols, err := validateMySQLCols(req.Columns)
		if err != nil {
			return err
		}
		if cname == "" {
			cname = "uq_" + tname + "_" + strings.Join(req.Columns, "_")
		}
		clause = fmt.Sprintf("UNIQUE (%s)", strings.Join(qCols, ", "))
	case "FOREIGN KEY":
		if len(req.Columns) == 0 || len(req.Columns) != len(req.RefColumns) {
			return fmt.Errorf("%w: columns and refColumns must match", engine.ErrInvalidInput)
		}
		qCols, err := validateMySQLCols(req.Columns)
		if err != nil {
			return err
		}
		qRefCols, err := validateMySQLCols(req.RefColumns)
		if err != nil {
			return err
		}
		rt, err := engine.ValidateName(req.RefTable)
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
			cname = "fk_" + tname + "_" + strings.Join(req.Columns, "_")
		}
		clause = fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s (%s)", strings.Join(qCols, ", "), quoteIdent(rt), strings.Join(qRefCols, ", "))
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
			cname = "chk_" + tname
		}
		clause = fmt.Sprintf("CHECK (%s)", expr)
	default:
		return fmt.Errorf("%w: unsupported constraint type %q", engine.ErrInvalidInput, req.Type)
	}
	cname, err = engine.ValidateName(cname)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s", quoteIdent(tname), quoteIdent(cname), clause))
	return err
}

func (d *Driver) DropConstraint(ctx context.Context, dbName, _, table, constraintName string) error {
	tname, err := engine.ValidateName(table)
	if err != nil {
		return err
	}
	cname, err := engine.ValidateName(constraintName)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	// MySQL differentiates DROP FOREIGN KEY vs DROP INDEX vs DROP CHECK
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", quoteIdent(tname), quoteIdent(cname)))
	if err != nil {
		if _, fkErr := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s", quoteIdent(tname), quoteIdent(cname))); fkErr == nil {
			return nil
		}
		if _, idxErr := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", quoteIdent(tname), quoteIdent(cname))); idxErr == nil {
			return nil
		}
	}
	return err
}

func validateMySQLCols(cols []string) ([]string, error) {
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

func (d *Driver) StreamRows(
	ctx context.Context,
	dbName string,
	req engine.QueryRowsReq,
	allRows bool,
	onHeader func([]engine.ColumnInfo) error,
	onRow func([]any) error,
) error {
	st, err := d.GetTableStructure(ctx, dbName, req.Schema, req.Table)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, dbName)
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
			ors = append(ors, fmt.Sprintf("CAST(%s AS CHAR) LIKE ?", quoteIdent(c.Name)))
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
			whereParts = append(whereParts, fmt.Sprintf("CAST(%s AS CHAR) LIKE ?", qc))
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
	} else if len(st.PrimaryKeys) > 0 {
		orderSQL = " ORDER BY " + quoteIdent(st.PrimaryKeys[0]) + " ASC"
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

	rows, err := db.QueryContext(ctx, sqlText, args...)
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
		for i := range raw {
			raw[i] = normalizeSQLValue(raw[i])
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
	st, err := d.GetTableStructure(ctx, dbName, dbName, table)
	if err != nil {
		return nil, err
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
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
			q = fmt.Sprintf("INSERT INTO %s () VALUES ()", quoteIdent(table))
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

func (d *Driver) GetActivityOverview(ctx context.Context, dbName string) (*engine.ActivityOverview, error) {
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	out := &engine.ActivityOverview{
		Metrics:       engine.ServerMetrics{CacheHitRatio: 100, MaxConnections: 151, TempBytesPretty: "0 B"},
		Sessions:      []engine.ActivitySession{},
		Locks:         []engine.LockInfo{},
		TableStats:    []engine.TablePerfInfo{},
		UnusedIndexes: []engine.UnusedIndexInfo{},
	}

	rows, err := db.QueryContext(ctx, `
		SELECT ID, COALESCE(USER,''), COALESCE(HOST,''), COALESCE(DB,''), COALESCE(COMMAND,''), COALESCE(TIME,0), COALESCE(STATE,''), COALESCE(INFO,'')
		FROM information_schema.PROCESSLIST
		ORDER BY TIME DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s engine.ActivitySession
			var cmd string
			var secs int64
			if rows.Scan(&s.PID, &s.Username, &s.ClientAddr, &s.Database, &cmd, &secs, &s.State, &s.Query) == nil {
				s.DurationMs = secs * 1000
				if s.State == "" {
					s.State = strings.ToLower(cmd)
				}
				if strings.EqualFold(cmd, "Query") {
					out.Metrics.ActiveCount++
				} else {
					out.Metrics.IdleCount++
				}
				out.Sessions = append(out.Sessions, s)
			}
		}
	}

	target := dbName
	if target == "" {
		target = d.cfg.Database
	}
	if target != "" {
		tRows, tErr := db.QueryContext(ctx, `
			SELECT TABLE_SCHEMA, TABLE_NAME, (COALESCE(DATA_LENGTH,0)+COALESCE(INDEX_LENGTH,0)), COALESCE(DATA_LENGTH,0), COALESCE(INDEX_LENGTH,0), COALESCE(TABLE_ROWS,0)
			FROM information_schema.TABLES
			WHERE TABLE_SCHEMA = ?
			ORDER BY (COALESCE(DATA_LENGTH,0)+COALESCE(INDEX_LENGTH,0)) DESC
		`, target)
		if tErr == nil {
			defer tRows.Close()
			for tRows.Next() {
				var tp engine.TablePerfInfo
				var tblB, idxB int64
				if tRows.Scan(&tp.Schema, &tp.Table, &tp.TotalSizeBytes, &tblB, &idxB, &tp.LiveTuples) == nil {
					tp.TotalSizePretty = engine.HumanBytes(tp.TotalSizeBytes)
					tp.TableSizePretty = engine.HumanBytes(tblB)
					tp.IndexSizePretty = engine.HumanBytes(idxB)
					out.TableStats = append(out.TableStats, tp)
				}
			}
		}
	}
	return out, nil
}

func (d *Driver) CancelBackend(ctx context.Context, pid int, terminate bool) error {
	if pid <= 0 {
		return fmt.Errorf("%w: invalid process id", engine.ErrInvalidInput)
	}
	db, err := d.getDB(ctx, "")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("KILL QUERY %d", pid)
	if terminate {
		cmd = fmt.Sprintf("KILL CONNECTION %d", pid)
	}
	_, err = db.ExecContext(ctx, cmd)
	return err
}

func (d *Driver) MaintainTable(ctx context.Context, dbName, _, table, action string) (*engine.SQLResult, error) {
	tname, err := engine.ValidateName(table)
	if err != nil {
		return nil, err
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	var q, tag string
	switch strings.ToLower(action) {
	case "analyze":
		q, tag = "ANALYZE TABLE "+quoteIdent(tname), "ANALYZE TABLE"
	case "vacuum", "vacuum_analyze", "reindex":
		q, tag = "OPTIMIZE TABLE "+quoteIdent(tname), "OPTIMIZE TABLE"
	default:
		return nil, engine.ErrUnsupported
	}
	_, err = db.ExecContext(ctx, q)
	if err != nil {
		return nil, err
	}
	return &engine.SQLResult{Columns: []string{}, Rows: [][]any{}, DurationMs: time.Since(started).Milliseconds(), CommandTag: tag}, nil
}

func (d *Driver) ExplainSQL(ctx context.Context, dbName, sqlText string, analyze bool) (*engine.ExplainResult, error) {
	clean := strings.TrimRight(strings.TrimSpace(sqlText), "; \t\r\n")
	if clean == "" {
		return nil, fmt.Errorf("%w: SQL query cannot be empty", engine.ErrInvalidInput)
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	if !analyze {
		var raw string
		if err := db.QueryRowContext(ctx, "EXPLAIN FORMAT=JSON "+clean).Scan(&raw); err == nil {
			var parsed any
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				return &engine.ExplainResult{PlanJSON: parsed, DurationMs: time.Since(started).Milliseconds(), Analyzed: false, Engine: engine.EngineMySQL}, nil
			}
			return &engine.ExplainResult{PlanLines: []string{raw}, DurationMs: time.Since(started).Milliseconds(), Analyzed: false, Engine: engine.EngineMySQL}, nil
		}
	}
	rows, err := db.QueryContext(ctx, "EXPLAIN "+clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var lines []string
	lines = append(lines, strings.Join(cols, " | "))
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if rows.Scan(ptrs...) == nil {
			var parts []string
			for _, v := range raw {
				parts = append(parts, fmt.Sprintf("%v", normalizeSQLValue(v)))
			}
			lines = append(lines, strings.Join(parts, " | "))
		}
	}
	return &engine.ExplainResult{PlanLines: lines, DurationMs: time.Since(started).Milliseconds(), Analyzed: analyze, Engine: engine.EngineMySQL}, nil
}

func (d *Driver) ListDatabaseObjects(ctx context.Context, dbName, _ string) (*engine.DatabaseObjects, error) {
	db, err := d.getDB(ctx, dbName)
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

	rRows, rErr := db.QueryContext(ctx, `
		SELECT ROUTINE_SCHEMA, ROUTINE_NAME, LOWER(ROUTINE_TYPE), COALESCE(DTD_IDENTIFIER,''), COALESCE(ROUTINE_DEFINITION,'')
		FROM information_schema.ROUTINES
		WHERE ROUTINE_SCHEMA = ?
		ORDER BY ROUTINE_NAME
	`, dbName)
	if rErr == nil {
		defer rRows.Close()
		for rRows.Next() {
			var fn engine.DBFunctionInfo
			if rRows.Scan(&fn.Schema, &fn.Name, &fn.Kind, &fn.ReturnType, &fn.Definition) == nil {
				fn.Language = "SQL"
				objs.Functions = append(objs.Functions, fn)
			}
		}
	}

	tRows, tErr := db.QueryContext(ctx, `
		SELECT TRIGGER_SCHEMA, TRIGGER_NAME, EVENT_OBJECT_TABLE, CONCAT(ACTION_TIMING, ' ', EVENT_MANIPULATION), COALESCE(ACTION_STATEMENT,'')
		FROM information_schema.TRIGGERS
		WHERE TRIGGER_SCHEMA = ?
		ORDER BY TRIGGER_NAME
	`, dbName)
	if tErr == nil {
		defer tRows.Close()
		for tRows.Next() {
			var tr engine.DBTriggerInfo
			tr.Enabled = true
			if tRows.Scan(&tr.Schema, &tr.Name, &tr.Table, &tr.Event, &tr.Definition) == nil {
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
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME, ORDINAL_POSITION
	`, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	order := []string{}
	byTable := make(map[string]*engine.AutocompleteTable)
	for rows.Next() {
		var s, t, c string
		if rows.Scan(&s, &t, &c) == nil {
			key := s + "." + t
			item, ok := byTable[key]
			if !ok {
				item = &engine.AutocompleteTable{Schema: s, Name: t, Type: "TABLE"}
				byTable[key] = item
				order = append(order, key)
			}
			item.Columns = append(item.Columns, c)
		}
	}
	out := make([]engine.AutocompleteTable, 0, len(order))
	for _, k := range order {
		out = append(out, *byTable[k])
	}
	return out, rows.Err()
}

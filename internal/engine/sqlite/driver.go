package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"pg-admin-pro/internal/engine"
)

// Driver implements engine.Driver for local SQLite databases.
type Driver struct {
	cfg    engine.ConnectionConfig
	db     *sql.DB
	dbName string
}

// New opens a SQLite database file and validates connectivity.
func New(ctx context.Context, cfg engine.ConnectionConfig) (*Driver, error) {
	path := strings.TrimSpace(cfg.Database)
	if path == "" {
		return nil, fmt.Errorf("%w: SQLite database file path is required", engine.ErrInvalidInput)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Driver{
		cfg:    cfg,
		db:     db,
		dbName: filepath.Base(path),
	}, nil
}

func (d *Driver) Kind() engine.EngineKind { return engine.EngineSQLite }

func (d *Driver) Capabilities() engine.Capabilities {
	return engine.Capabilities{
		MultiDatabase:   false,
		UserManagement:  false,
		Schemas:         false,
		BinaryBackup:    false,
		RoleAssignment:  false,
		ActivityMonitor: false,
		ObjectsBrowser:  true,
		ExplainPlan:     true,
	}
}

func (d *Driver) Config() engine.PublicConnectionInfo {
	return engine.PublicConnectionInfo{
		Engine:   engine.EngineSQLite,
		Host:     "local-file",
		Database: d.cfg.Database,
		Username: "sqlite",
	}
}

func (d *Driver) Close() error                       { return d.db.Close() }
func (d *Driver) Ping(ctx context.Context) error     { return d.db.PingContext(ctx) }
func quoteIdent(s string) string                     { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func (d *Driver) Overview(ctx context.Context) (*engine.ServerOverview, error) {
	var ver string
	if err := d.db.QueryRowContext(ctx, `SELECT sqlite_version()`).Scan(&ver); err != nil {
		return nil, err
	}
	var sizeBytes int64
	if fi, err := os.Stat(d.cfg.Database); err == nil {
		sizeBytes = fi.Size()
	}
	return &engine.ServerOverview{
		Engine:          engine.EngineSQLite,
		Version:         "SQLite " + ver,
		CurrentUser:     "sqlite",
		CurrentDatabase: d.dbName,
		IsAdmin:         true,
		TotalDatabases:  1,
		TotalUsers:      1,
		ActiveConns:     1,
		TotalSizeBytes:  sizeBytes,
		TotalSizePretty: engine.HumanBytes(sizeBytes),
		Capabilities:    d.Capabilities(),
	}, nil
}

func (d *Driver) ListDatabases(ctx context.Context) ([]engine.DatabaseInfo, error) {
	var sizeBytes int64
	if fi, err := os.Stat(d.cfg.Database); err == nil {
		sizeBytes = fi.Size()
	}
	return []engine.DatabaseInfo{{
		Name:          d.dbName,
		Owner:         "local",
		Encoding:      "UTF-8",
		SizeBytes:     sizeBytes,
		SizePretty:    engine.HumanBytes(sizeBytes),
		ActiveConns:   1,
		AllowConnect:  true,
		AssignedUsers: []string{"local"},
	}}, nil
}

func (d *Driver) CreateDatabase(context.Context, engine.CreateDatabaseReq) error {
	return engine.ErrUnsupported
}
func (d *Driver) DropDatabase(context.Context, string) error { return engine.ErrUnsupported }
func (d *Driver) ListUsers(context.Context) ([]engine.UserInfo, error) {
	return []engine.UserInfo{}, nil
}
func (d *Driver) CreateUser(context.Context, engine.CreateUserReq) error {
	return engine.ErrUnsupported
}
func (d *Driver) UpdateUser(context.Context, string, engine.UpdateUserReq) error {
	return engine.ErrUnsupported
}
func (d *Driver) DropUser(context.Context, string) error { return engine.ErrUnsupported }
func (d *Driver) ListDatabaseGrants(context.Context, string) ([]engine.DatabaseGrant, error) {
	return []engine.DatabaseGrant{}, nil
}
func (d *Driver) AssignUserToDatabase(context.Context, string, engine.AssignUserReq) error {
	return engine.ErrUnsupported
}
func (d *Driver) RevokeUserFromDatabase(context.Context, string, string) error {
	return engine.ErrUnsupported
}

func (d *Driver) ListSchemas(ctx context.Context, _ string, _ bool) ([]engine.SchemaInfo, error) {
	var cnt int
	_ = d.db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'`).Scan(&cnt)
	return []engine.SchemaInfo{{Name: "main", Owner: "local", TableCount: cnt}}, nil
}

func (d *Driver) ListTables(ctx context.Context, _, _ string) ([]engine.TableInfo, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		return nil, err
	}

	type rawTbl struct{ name, typ string }
	var raw []rawTbl
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			rows.Close()
			return nil, err
		}
		raw = append(raw, rawTbl{name: name, typ: typ})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	out := make([]engine.TableInfo, 0, len(raw))
	for _, r := range raw {
		var cnt int64
		_ = d.db.QueryRowContext(ctx, "SELECT count(*) FROM "+quoteIdent(r.name)).Scan(&cnt)
		out = append(out, engine.TableInfo{
			Schema:      "main",
			Name:        r.name,
			Type:        strings.ToUpper(r.typ),
			RowEstimate: cnt,
			TotalSize:   "-",
		})
	}
	return out, nil
}

func (d *Driver) GetTableStructure(ctx context.Context, _, _, table string) (*engine.TableStructure, error) {
	rows, err := d.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteIdent(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []engine.ColumnInfo
	var pks []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defVal sql.NullString
		var pkOrder int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defVal, &pkOrder); err != nil {
			return nil, err
		}
		if typ == "" {
			typ = "TEXT"
		}
		c := engine.ColumnInfo{
			OrdinalPos:   cid + 1,
			Name:         name,
			DataType:     typ,
			FormatType:   typ,
			IsNullable:   notNull == 0,
			IsPrimaryKey: pkOrder > 0,
		}
		if defVal.Valid {
			s := defVal.String
			c.DefaultValue = &s
			c.HasDefault = true
		}
		if c.IsPrimaryKey {
			pks = append(pks, name)
		}
		cols = append(cols, c)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table %q not found", table)
	}
	if pks == nil {
		pks = []string{}
	}

	// Foreign keys
	fkRows, fkErr := d.db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", quoteIdent(table)))
	if fkErr == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var id, seq int
			var refTable, fromCol, toCol, onUpd, onDel, match string
			if scanErr := fkRows.Scan(&id, &seq, &refTable, &fromCol, &toCol, &onUpd, &onDel, &match); scanErr == nil {
				for i := range cols {
					if cols[i].Name == fromCol {
						cols[i].ForeignKey = &engine.ForeignKeyRef{
							ConstraintName: fmt.Sprintf("fk_%s_%d", table, id),
							RefSchema:      "main",
							RefTable:       refTable,
							RefColumn:      toCol,
						}
					}
				}
			}
		}
	}

	// Indexes
	indexes := []engine.IndexInfo{}
	idxRows, idxErr := d.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", quoteIdent(table)))
	if idxErr == nil {
		type rawIdx struct {
			name   string
			unique bool
			origin string
		}
		var raws []rawIdx
		for idxRows.Next() {
			var seq int
			var name, origin string
			var uniq, partial int
			if scanErr := idxRows.Scan(&seq, &name, &uniq, &origin, &partial); scanErr == nil {
				raws = append(raws, rawIdx{name: name, unique: uniq != 0, origin: origin})
			}
		}
		idxRows.Close()
		for _, r := range raws {
			var colNames []string
			ir, irErr := d.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%s)", quoteIdent(r.name)))
			if irErr == nil {
				for ir.Next() {
					var s, cid int
					var cname sql.NullString
					if ir.Scan(&s, &cid, &cname) == nil && cname.Valid {
						colNames = append(colNames, cname.String)
					}
				}
				ir.Close()
			}
			indexes = append(indexes, engine.IndexInfo{
				Name:       r.name,
				IsUnique:   r.unique,
				IsPrimary:  r.origin == "pk",
				Columns:    strings.Join(colNames, ", "),
				Definition: fmt.Sprintf("INDEX %s (%s)", r.name, strings.Join(colNames, ", ")),
			})
		}
	}

	return &engine.TableStructure{
		Schema:            "main",
		Table:             table,
		Type:              "BASE TABLE",
		Columns:           cols,
		Indexes:           indexes,
		Constraints:       []engine.ConstraintInfo{},
		PrimaryKeys:       pks,
		UsesRowIDFallback: len(pks) == 0,
	}, nil
}

func (d *Driver) CreateTable(ctx context.Context, _ string, req engine.CreateTableReq) error {
	tname, err := engine.ValidateName(req.Table)
	if err != nil {
		return err
	}
	if len(req.Columns) == 0 {
		return fmt.Errorf("%w: at least one column required", engine.ErrInvalidInput)
	}
	var defs []string
	var pks []string
	for _, c := range req.Columns {
		cname, err := engine.ValidateName(c.Name)
		if err != nil {
			return err
		}
		ctype, err := engine.ValidateColumnType(c.Type)
		if err != nil {
			return err
		}
		def := quoteIdent(cname) + " " + ctype
		if !c.Nullable && !c.PrimaryKey {
			def += " NOT NULL"
		}
		if dflt, err := engine.ValidateDefaultExpr(c.Default); err != nil {
			return err
		} else if dflt != "" {
			def += " DEFAULT " + dflt
		}
		defs = append(defs, def)
		if c.PrimaryKey {
			pks = append(pks, quoteIdent(cname))
		}
	}
	if len(pks) > 0 {
		defs = append(defs, "PRIMARY KEY ("+strings.Join(pks, ", ")+")")
	}
	_, err = d.db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdent(tname), strings.Join(defs, ", ")))
	return err
}

func (d *Driver) DropTable(ctx context.Context, _, _, table string) error {
	_, err := d.db.ExecContext(ctx, "DROP TABLE "+quoteIdent(table))
	return err
}

func (d *Driver) QueryRows(ctx context.Context, dbName string, req engine.QueryRowsReq) (*engine.QueryRowsResp, error) {
	started := time.Now()
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 500 {
		req.PageSize = 50
	}
	st, err := d.GetTableStructure(ctx, dbName, "main", req.Table)
	if err != nil {
		return nil, err
	}
	colMap := make(map[string]engine.ColumnInfo, len(st.Columns))
	var selectCols []string
	for _, c := range st.Columns {
		colMap[c.Name] = c
		selectCols = append(selectCols, quoteIdent(c.Name))
	}
	if st.UsesRowIDFallback {
		selectCols = append(selectCols, "rowid")
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
			return nil, fmt.Errorf("%w: unknown column %q", engine.ErrInvalidInput, f.Column)
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
	var total int64
	if err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM "+quoteIdent(req.Table)+whereSQL, args...).Scan(&total); err != nil {
		return nil, err
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
	offset := (req.Page - 1) * req.PageSize
	dataArgs := append(append([]any{}, args...), req.PageSize, offset)
	rows, err := d.db.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s%s%s LIMIT ? OFFSET ?", strings.Join(selectCols, ", "), quoteIdent(req.Table), whereSQL, orderSQL), dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scanCount := len(selectCols)
	resRows := make([]map[string]any, 0)
	locs := make([]map[string]any, 0)
	for rows.Next() {
		raw := make([]any, scanCount)
		ptrs := make([]any, scanCount)
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(st.Columns))
		for i, c := range st.Columns {
			if b, ok := raw[i].([]byte); ok {
				m[c.Name] = string(b)
			} else {
				m[c.Name] = raw[i]
			}
		}
		resRows = append(resRows, m)
		loc := make(map[string]any)
		if st.UsesRowIDFallback {
			loc["__rowid"] = raw[len(st.Columns)]
		} else {
			for _, pk := range st.PrimaryKeys {
				loc[pk] = m[pk]
			}
		}
		locs = append(locs, loc)
	}
	return &engine.QueryRowsResp{
		Columns:           st.Columns,
		PrimaryKeys:       st.PrimaryKeys,
		UsesRowIDFallback: st.UsesRowIDFallback,
		Rows:              resRows,
		RowLocators:       locs,
		TotalCount:        total,
		Page:              req.Page,
		PageSize:          req.PageSize,
		DurationMs:        time.Since(started).Milliseconds(),
	}, rows.Err()
}

func (d *Driver) InsertRow(ctx context.Context, dbName, _, table string, values map[string]any) (map[string]any, error) {
	st, err := d.GetTableStructure(ctx, dbName, "main", table)
	if err != nil {
		return nil, err
	}
	var cols, phs []string
	var args []any
	for _, c := range st.Columns {
		if v, ok := values[c.Name]; ok {
			cols = append(cols, quoteIdent(c.Name))
			phs = append(phs, "?")
			args = append(args, v)
		}
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteIdent(table), strings.Join(cols, ", "), strings.Join(phs, ", "))
	if len(cols) == 0 {
		q = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", quoteIdent(table))
	}
	if _, err := d.db.ExecContext(ctx, q, args...); err != nil {
		return nil, err
	}
	return values, nil
}

func (d *Driver) UpdateRow(ctx context.Context, dbName, _, table string, locator, updates map[string]any) (map[string]any, error) {
	st, err := d.GetTableStructure(ctx, dbName, "main", table)
	if err != nil {
		return nil, err
	}
	var sets []string
	var args []any
	for _, c := range st.Columns {
		if v, ok := updates[c.Name]; ok {
			sets = append(sets, quoteIdent(c.Name)+" = ?")
			args = append(args, v)
		}
	}
	whereClause, whereArgs, err := sqliteWhere(st, locator)
	if err != nil {
		return nil, err
	}
	args = append(args, whereArgs...)
	if _, err := d.db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s WHERE %s", quoteIdent(table), strings.Join(sets, ", "), whereClause), args...); err != nil {
		return nil, err
	}
	return updates, nil
}

func (d *Driver) DeleteRows(ctx context.Context, dbName, _, table string, locators []map[string]any) (int64, error) {
	st, err := d.GetTableStructure(ctx, dbName, "main", table)
	if err != nil {
		return 0, err
	}
	var deleted int64
	for _, loc := range locators {
		w, wArgs, err := sqliteWhere(st, loc)
		if err != nil {
			return deleted, err
		}
		res, err := d.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", quoteIdent(table), w), wArgs...)
		if err != nil {
			return deleted, err
		}
		n, _ := res.RowsAffected()
		deleted += n
	}
	return deleted, nil
}

func sqliteWhere(st *engine.TableStructure, loc map[string]any) (string, []any, error) {
	if st.UsesRowIDFallback {
		v, ok := loc["__rowid"]
		if !ok {
			return "", nil, fmt.Errorf("%w: missing __rowid", engine.ErrInvalidInput)
		}
		return "rowid = ?", []any{v}, nil
	}
	var parts []string
	var args []any
	for _, pk := range st.PrimaryKeys {
		v, ok := loc[pk]
		if !ok {
			return "", nil, fmt.Errorf("%w: missing pk %q", engine.ErrInvalidInput, pk)
		}
		parts = append(parts, quoteIdent(pk)+" = ?")
		args = append(args, v)
	}
	return strings.Join(parts, " AND "), args, nil
}

func (d *Driver) ExecuteSQL(ctx context.Context, _, sqlText string) (*engine.SQLResult, error) {
	started := time.Now()
	trimmed := strings.TrimSpace(strings.ToUpper(sqlText))
	isQuery := strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "PRAGMA") || strings.HasPrefix(trimmed, "EXPLAIN") || strings.HasPrefix(trimmed, "WITH")
	if !isQuery {
		res, err := d.db.ExecContext(ctx, sqlText)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		return &engine.SQLResult{
			Columns:      []string{},
			Rows:         [][]any{},
			RowsAffected: n,
			DurationMs:   time.Since(started).Milliseconds(),
			CommandTag:   fmt.Sprintf("OK (%d rows affected)", n),
		}, nil
	}
	rows, err := d.db.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	res := make([][]any, 0)
	for rows.Next() && len(res) < 2000 {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range raw {
			if b, ok := v.([]byte); ok {
				raw[i] = string(b)
			}
		}
		res = append(res, raw)
	}
	return &engine.SQLResult{
		Columns:    cols,
		Rows:       res,
		DurationMs: time.Since(started).Milliseconds(),
		CommandTag: fmt.Sprintf("SELECT %d", len(res)),
		IsSelect:   true,
	}, rows.Err()
}

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	mysqldrv "github.com/go-sql-driver/mysql"

	"pg-admin-pro/internal/engine"
)

// Driver implements engine.Driver for MySQL and MariaDB.
type Driver struct {
	cfg   engine.ConnectionConfig
	mu    sync.Mutex
	pools map[string]*sql.DB
}

// New creates a MySQL/MariaDB driver and verifies the connection via Ping.
func New(ctx context.Context, cfg engine.ConnectionConfig) (*Driver, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 3306
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "false"
	}

	d := &Driver{
		cfg:   cfg,
		pools: make(map[string]*sql.DB),
	}
	if err := d.Ping(ctx); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

func (d *Driver) Kind() engine.EngineKind { return engine.EngineMySQL }

func (d *Driver) Capabilities() engine.Capabilities {
	return engine.Capabilities{
		MultiDatabase:   true,
		UserManagement:  true,
		Schemas:         false,
		BinaryBackup:    false,
		RoleAssignment:  true,
		ActivityMonitor: true,
		ObjectsBrowser:  true,
		ExplainPlan:     true,
	}
}

func (d *Driver) Config() engine.PublicConnectionInfo {
	return engine.PublicConnectionInfo{
		Engine:   engine.EngineMySQL,
		Host:     d.cfg.Host,
		Port:     d.cfg.Port,
		Database: d.cfg.Database,
		Username: d.cfg.Username,
		SSLMode:  d.cfg.SSLMode,
	}
}

func (d *Driver) RawConfig() engine.ConnectionConfig { return d.cfg }

func (d *Driver) getDB(ctx context.Context, dbName string) (*sql.DB, error) {
	target := strings.TrimSpace(dbName)
	if target == "" {
		target = d.cfg.Database
	}

	d.mu.Lock()
	if db, ok := d.pools[target]; ok {
		d.mu.Unlock()
		return db, nil
	}
	d.mu.Unlock()

	mc := mysqldrv.NewConfig()
	mc.User = d.cfg.Username
	mc.Passwd = d.cfg.Password
	mc.Net = "tcp"
	mc.Addr = fmt.Sprintf("%s:%d", d.cfg.Host, d.cfg.Port)
	mc.DBName = target
	mc.ParseTime = true
	mc.Timeout = 8 * time.Second
	mc.ReadTimeout = 30 * time.Second
	mc.WriteTimeout = 30 * time.Second

	db, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(2 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if existing, ok := d.pools[target]; ok {
		_ = db.Close()
		return existing, nil
	}
	d.pools[target] = db
	return db, nil
}

func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, db := range d.pools {
		_ = db.Close()
		delete(d.pools, k)
	}
	return nil
}

func (d *Driver) Ping(ctx context.Context) error {
	db, err := d.getDB(ctx, d.cfg.Database)
	if err != nil {
		return err
	}
	return db.PingContext(ctx)
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func quoteLiteral(s string) (string, error) {
	if strings.ContainsRune(s, '\x00') {
		return "", fmt.Errorf("invalid null byte")
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `''`)
	return "'" + escaped + "'", nil
}

func (d *Driver) Overview(ctx context.Context) (*engine.ServerOverview, error) {
	db, err := d.getDB(ctx, "")
	if err != nil {
		return nil, err
	}
	ov := &engine.ServerOverview{
		Engine:       engine.EngineMySQL,
		Capabilities: d.Capabilities(),
	}
	var currentDB sql.NullString
	err = db.QueryRowContext(ctx, `SELECT VERSION(), CURRENT_USER(), DATABASE()`).Scan(&ov.Version, &ov.CurrentUser, &currentDB)
	if err != nil {
		return nil, err
	}
	ov.CurrentDatabase = currentDB.String
	ov.IsAdmin = true

	_ = db.QueryRowContext(ctx, `
		SELECT count(*), COALESCE(sum(size_bytes), 0)
		FROM (
			SELECT s.SCHEMA_NAME, COALESCE(sum(t.DATA_LENGTH + t.INDEX_LENGTH), 0) AS size_bytes
			FROM information_schema.SCHEMATA s
			LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = s.SCHEMA_NAME
			WHERE s.SCHEMA_NAME NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys')
			GROUP BY s.SCHEMA_NAME
		) x
	`).Scan(&ov.TotalDatabases, &ov.TotalSizeBytes)
	ov.TotalSizePretty = engine.HumanBytes(ov.TotalSizeBytes)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM mysql.user`).Scan(&ov.TotalUsers)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.PROCESSLIST`).Scan(&ov.ActiveConns)
	return ov, nil
}

func (d *Driver) ListDatabases(ctx context.Context) ([]engine.DatabaseInfo, error) {
	db, err := d.getDB(ctx, "")
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT
			s.SCHEMA_NAME,
			s.DEFAULT_CHARACTER_SET_NAME,
			s.DEFAULT_COLLATION_NAME,
			COALESCE(sum(t.DATA_LENGTH + t.INDEX_LENGTH), 0) AS size_bytes
		FROM information_schema.SCHEMATA s
		LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = s.SCHEMA_NAME
		WHERE s.SCHEMA_NAME NOT IN ('information_schema', 'performance_schema', 'sys')
		GROUP BY s.SCHEMA_NAME, s.DEFAULT_CHARACTER_SET_NAME, s.DEFAULT_COLLATION_NAME
		ORDER BY s.SCHEMA_NAME
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dbs []engine.DatabaseInfo
	for rows.Next() {
		var item engine.DatabaseInfo
		if err := rows.Scan(&item.Name, &item.Encoding, &item.Collation, &item.SizeBytes); err != nil {
			return nil, err
		}
		item.SizePretty = engine.HumanBytes(item.SizeBytes)
		item.AllowConnect = true
		item.Owner = "root"
		item.AssignedUsers = []string{}
		dbs = append(dbs, item)
	}
	if dbs == nil {
		dbs = []engine.DatabaseInfo{}
	}
	return dbs, rows.Err()
}

func (d *Driver) CreateDatabase(ctx context.Context, req engine.CreateDatabaseReq) error {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return fmt.Errorf("%w: database name required", engine.ErrInvalidInput)
	}
	db, err := d.getDB(ctx, "")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(name)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

func (d *Driver) DropDatabase(ctx context.Context, dbName string) error {
	if dbName == "" || dbName == "mysql" || dbName == d.cfg.Database {
		return fmt.Errorf("%w: cannot drop protected database %q", engine.ErrInvalidInput, dbName)
	}
	d.mu.Lock()
	if pool, ok := d.pools[dbName]; ok {
		_ = pool.Close()
		delete(d.pools, dbName)
	}
	d.mu.Unlock()

	db, err := d.getDB(ctx, "")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "DROP DATABASE "+quoteIdent(dbName))
	return err
}

func (d *Driver) ListUsers(ctx context.Context) ([]engine.UserInfo, error) {
	db, err := d.getDB(ctx, "")
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT CONCAT(User, '@', Host), (Super_priv = 'Y'), (account_locked = 'N'), (Create_user_priv = 'Y')
		FROM mysql.user
		WHERE User <> ''
		ORDER BY User, Host
	`)
	if err != nil {
		return nil, fmt.Errorf("list mysql users: %w", err)
	}
	defer rows.Close()

	var users []engine.UserInfo
	for rows.Next() {
		var u engine.UserInfo
		if err := rows.Scan(&u.Name, &u.IsSuperuser, &u.CanLogin, &u.CanCreateRole); err != nil {
			return nil, err
		}
		u.CanCreateDB = u.IsSuperuser
		u.ConnLimit = -1
		u.OwnedDatabases = []string{}
		users = append(users, u)
	}
	if users == nil {
		users = []engine.UserInfo{}
	}
	return users, rows.Err()
}

func parseMySQLAccount(name string) (string, string) {
	if idx := strings.LastIndex(name, "@"); idx != -1 {
		return name[:idx], name[idx+1:]
	}
	return name, "%"
}

func (d *Driver) CreateUser(ctx context.Context, req engine.CreateUserReq) error {
	user, host := parseMySQLAccount(strings.TrimSpace(req.Username))
	if user == "" {
		return fmt.Errorf("%w: username required", engine.ErrInvalidInput)
	}
	uLit, err := quoteLiteral(user)
	if err != nil {
		return err
	}
	hLit, err := quoteLiteral(host)
	if err != nil {
		return err
	}
	pLit, err := quoteLiteral(req.Password)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, "")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE USER %s@%s IDENTIFIED BY %s", uLit, hLit, pLit))
	return err
}

func (d *Driver) UpdateUser(ctx context.Context, username string, req engine.UpdateUserReq) error {
	if req.Password == nil || *req.Password == "" {
		return nil
	}
	user, host := parseMySQLAccount(username)
	uLit, err := quoteLiteral(user)
	if err != nil {
		return err
	}
	hLit, err := quoteLiteral(host)
	if err != nil {
		return err
	}
	pLit, err := quoteLiteral(*req.Password)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, "")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER USER %s@%s IDENTIFIED BY %s", uLit, hLit, pLit))
	return err
}

func (d *Driver) DropUser(ctx context.Context, username string) error {
	user, host := parseMySQLAccount(username)
	uLit, err := quoteLiteral(user)
	if err != nil {
		return err
	}
	hLit, err := quoteLiteral(host)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, "")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("DROP USER %s@%s", uLit, hLit))
	return err
}

func (d *Driver) ListDatabaseGrants(ctx context.Context, dbName string) ([]engine.DatabaseGrant, error) {
	db, err := d.getDB(ctx, "")
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT CONCAT(u.User, '@', u.Host),
		       COALESCE(MAX(d.Select_priv = 'Y'), 0),
		       COALESCE(MAX(d.Insert_priv = 'Y'), 0),
		       COALESCE(MAX(d.Create_priv = 'Y'), 0)
		FROM mysql.user u
		LEFT JOIN mysql.db d ON d.User = u.User AND d.Host = u.Host AND d.Db = ?
		WHERE u.User <> ''
		GROUP BY u.User, u.Host
		ORDER BY u.User, u.Host
	`, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grants []engine.DatabaseGrant
	for rows.Next() {
		var g engine.DatabaseGrant
		var canSel, canIns, canCreate bool
		if err := rows.Scan(&g.Username, &canSel, &canIns, &canCreate); err != nil {
			return nil, err
		}
		g.CanConnect = canSel || canIns || canCreate
		g.CanCreate = canCreate
		if canCreate {
			g.SchemaAccess = "admin"
		} else if canIns {
			g.SchemaAccess = "read_write"
		} else if canSel {
			g.SchemaAccess = "read_only"
		} else {
			g.SchemaAccess = "none"
		}
		grants = append(grants, g)
	}
	if grants == nil {
		grants = []engine.DatabaseGrant{}
	}
	return grants, rows.Err()
}

func (d *Driver) AssignUserToDatabase(ctx context.Context, dbName string, req engine.AssignUserReq) error {
	user, host := parseMySQLAccount(req.Username)
	uLit, err := quoteLiteral(user)
	if err != nil {
		return err
	}
	hLit, err := quoteLiteral(host)
	if err != nil {
		return err
	}
	privs := "SELECT"
	switch req.Preset {
	case "owner", "admin":
		privs = "ALL PRIVILEGES"
	case "read_write":
		privs = "SELECT, INSERT, UPDATE, DELETE"
	case "read_only":
		privs = "SELECT"
	}

	db, err := d.getDB(ctx, "")
	if err != nil {
		return err
	}
	_, _ = db.ExecContext(ctx, fmt.Sprintf("REVOKE ALL PRIVILEGES ON %s.* FROM %s@%s", quoteIdent(dbName), uLit, hLit))
	if _, err := db.ExecContext(ctx, fmt.Sprintf("GRANT %s ON %s.* TO %s@%s", privs, quoteIdent(dbName), uLit, hLit)); err != nil {
		return err
	}
	_, _ = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	return nil
}

func (d *Driver) RevokeUserFromDatabase(ctx context.Context, dbName, username string) error {
	user, host := parseMySQLAccount(username)
	uLit, err := quoteLiteral(user)
	if err != nil {
		return err
	}
	hLit, err := quoteLiteral(host)
	if err != nil {
		return err
	}
	db, err := d.getDB(ctx, "")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("REVOKE ALL PRIVILEGES ON %s.* FROM %s@%s", quoteIdent(dbName), uLit, hLit))
	return err
}

func (d *Driver) ListSchemas(ctx context.Context, dbName string, _ bool) ([]engine.SchemaInfo, error) {
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	var cnt int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ?`, dbName).Scan(&cnt)
	return []engine.SchemaInfo{{Name: dbName, Owner: d.cfg.Username, TableCount: cnt}}, nil
}

func (d *Driver) ListTables(ctx context.Context, dbName, _ string) ([]engine.TableInfo, error) {
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT
			TABLE_SCHEMA,
			TABLE_NAME,
			TABLE_TYPE,
			COALESCE(TABLE_ROWS, 0),
			COALESCE(DATA_LENGTH + INDEX_LENGTH, 0),
			COALESCE(TABLE_COMMENT, '')
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_TYPE, TABLE_NAME
	`, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []engine.TableInfo
	for rows.Next() {
		var t engine.TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.Type, &t.RowEstimate, &t.SizeBytes, &t.Comment); err != nil {
			return nil, err
		}
		t.TotalSize = engine.HumanBytes(t.SizeBytes)
		tables = append(tables, t)
	}
	if tables == nil {
		tables = []engine.TableInfo{}
	}
	return tables, rows.Err()
}

func (d *Driver) GetTableStructure(ctx context.Context, dbName, _, table string) (*engine.TableStructure, error) {
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT
			ORDINAL_POSITION,
			COLUMN_NAME,
			DATA_TYPE,
			COLUMN_TYPE,
			(IS_NULLABLE = 'YES'),
			COLUMN_DEFAULT,
			(COLUMN_KEY = 'PRI'),
			(EXTRA LIKE '%auto_increment%')
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`, dbName, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []engine.ColumnInfo
	var pks []string
	for rows.Next() {
		var c engine.ColumnInfo
		var defVal sql.NullString
		if err := rows.Scan(&c.OrdinalPos, &c.Name, &c.DataType, &c.FormatType, &c.IsNullable, &defVal, &c.IsPrimaryKey, &c.IsIdentity); err != nil {
			return nil, err
		}
		if defVal.Valid {
			s := defVal.String
			c.DefaultValue = &s
			c.HasDefault = true
		}
		if c.IsPrimaryKey {
			pks = append(pks, c.Name)
		}
		if strings.HasPrefix(strings.ToLower(c.FormatType), "enum(") {
			c.EnumValues = parseMySQLEnum(c.FormatType)
		}
		cols = append(cols, c)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("table %s.%s not found", dbName, table)
	}
	if pks == nil {
		pks = []string{}
	}

	indexes := []engine.IndexInfo{}
	idxRows, idxErr := db.QueryContext(ctx, `
		SELECT INDEX_NAME, (NON_UNIQUE = 0) AS is_unique, GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ', ') AS cols
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		GROUP BY INDEX_NAME, NON_UNIQUE
		ORDER BY (INDEX_NAME = 'PRIMARY') DESC, INDEX_NAME
	`, dbName, table)
	if idxErr == nil {
		defer idxRows.Close()
		for idxRows.Next() {
			var idx engine.IndexInfo
			if scanErr := idxRows.Scan(&idx.Name, &idx.IsUnique, &idx.Columns); scanErr == nil {
				idx.IsPrimary = idx.Name == "PRIMARY"
				idx.Definition = fmt.Sprintf("INDEX %s (%s)", quoteIdent(idx.Name), idx.Columns)
				indexes = append(indexes, idx)
			}
		}
	}

	constraints := []engine.ConstraintInfo{}
	cRows, cErr := db.QueryContext(ctx, `
		SELECT CONSTRAINT_NAME, CONSTRAINT_TYPE
		FROM information_schema.TABLE_CONSTRAINTS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY CONSTRAINT_TYPE, CONSTRAINT_NAME
	`, dbName, table)
	if cErr == nil {
		defer cRows.Close()
		for cRows.Next() {
			var con engine.ConstraintInfo
			if scanErr := cRows.Scan(&con.Name, &con.Type); scanErr == nil {
				con.Definition = con.Type + " " + con.Name
				constraints = append(constraints, con)
			}
		}
	}

	return &engine.TableStructure{
		Schema:      dbName,
		Table:       table,
		Type:        "BASE TABLE",
		Columns:     cols,
		Indexes:     indexes,
		Constraints: constraints,
		PrimaryKeys: pks,
	}, nil
}

func (d *Driver) CreateTable(ctx context.Context, dbName string, req engine.CreateTableReq) error {
	tname, err := engine.ValidateName(req.Table)
	if err != nil {
		return err
	}
	if len(req.Columns) == 0 {
		return fmt.Errorf("%w: at least one column required", engine.ErrInvalidInput)
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
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
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdent(tname), strings.Join(defs, ", ")))
	return err
}

func (d *Driver) DropTable(ctx context.Context, dbName, _, table string) error {
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "DROP TABLE "+quoteIdent(table))
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
	st, err := d.GetTableStructure(ctx, dbName, req.Schema, req.Table)
	if err != nil {
		return nil, err
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}

	colMap := make(map[string]engine.ColumnInfo, len(st.Columns))
	var selectCols []string
	for _, c := range st.Columns {
		colMap[c.Name] = c
		selectCols = append(selectCols, quoteIdent(c.Name))
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

	var totalCount int64
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+quoteIdent(req.Table)+whereSQL, args...).Scan(&totalCount); err != nil {
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
	} else if len(st.PrimaryKeys) > 0 {
		orderSQL = " ORDER BY " + quoteIdent(st.PrimaryKeys[0]) + " ASC"
	}

	offset := (req.Page - 1) * req.PageSize
	dataArgs := append(append([]any{}, args...), req.PageSize, offset)
	sqlText := fmt.Sprintf("SELECT %s FROM %s%s%s LIMIT ? OFFSET ?", strings.Join(selectCols, ", "), quoteIdent(req.Table), whereSQL, orderSQL)
	rows, err := db.QueryContext(ctx, sqlText, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resRows, locators, err := scanSQLRowsToMaps(rows, st.Columns, st.PrimaryKeys)
	if err != nil {
		return nil, err
	}

	return &engine.QueryRowsResp{
		Columns:     st.Columns,
		PrimaryKeys: st.PrimaryKeys,
		Rows:        resRows,
		RowLocators: locators,
		TotalCount:  totalCount,
		Page:        req.Page,
		PageSize:    req.PageSize,
		DurationMs:  time.Since(started).Milliseconds(),
	}, nil
}

func (d *Driver) InsertRow(ctx context.Context, dbName, _, table string, values map[string]any) (map[string]any, error) {
	st, err := d.GetTableStructure(ctx, dbName, dbName, table)
	if err != nil {
		return nil, err
	}
	db, err := d.getDB(ctx, dbName)
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
		q = fmt.Sprintf("INSERT INTO %s () VALUES ()", quoteIdent(table))
	}
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		return nil, err
	}
	return values, nil
}

func (d *Driver) UpdateRow(ctx context.Context, dbName, _, table string, locator, updates map[string]any) (map[string]any, error) {
	st, err := d.GetTableStructure(ctx, dbName, dbName, table)
	if err != nil {
		return nil, err
	}
	if len(st.PrimaryKeys) == 0 {
		return nil, fmt.Errorf("%w: MySQL table requires a primary key for updates", engine.ErrInvalidInput)
	}
	db, err := d.getDB(ctx, dbName)
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
	var wheres []string
	for _, pk := range st.PrimaryKeys {
		v, ok := locator[pk]
		if !ok {
			return nil, fmt.Errorf("%w: missing pk %q", engine.ErrInvalidInput, pk)
		}
		wheres = append(wheres, quoteIdent(pk)+" = ?")
		args = append(args, v)
	}
	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s", quoteIdent(table), strings.Join(sets, ", "), strings.Join(wheres, " AND "))
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		return nil, err
	}
	return updates, nil
}

func (d *Driver) DeleteRows(ctx context.Context, dbName, _, table string, locators []map[string]any) (int64, error) {
	st, err := d.GetTableStructure(ctx, dbName, dbName, table)
	if err != nil {
		return 0, err
	}
	if len(st.PrimaryKeys) == 0 {
		return 0, fmt.Errorf("%w: MySQL table requires a primary key for deletes", engine.ErrInvalidInput)
	}
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return 0, err
	}
	var deleted int64
	for _, loc := range locators {
		var wheres []string
		var args []any
		for _, pk := range st.PrimaryKeys {
			wheres = append(wheres, quoteIdent(pk)+" = ?")
			args = append(args, loc[pk])
		}
		res, err := db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", quoteIdent(table), strings.Join(wheres, " AND ")), args...)
		if err != nil {
			return deleted, err
		}
		n, _ := res.RowsAffected()
		deleted += n
	}
	return deleted, nil
}

func (d *Driver) ExecuteSQL(ctx context.Context, dbName, sqlText string) (*engine.SQLResult, error) {
	db, err := d.getDB(ctx, dbName)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	trimmed := strings.TrimSpace(strings.ToUpper(sqlText))
	isQuery := strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "SHOW") ||
		strings.HasPrefix(trimmed, "DESCRIBE") || strings.HasPrefix(trimmed, "EXPLAIN") || strings.HasPrefix(trimmed, "WITH")
	if !isQuery {
		res, err := db.ExecContext(ctx, sqlText)
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
			IsSelect:     false,
		}, nil
	}
	rows, err := db.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	resultRows := make([][]any, 0)
	for rows.Next() && len(resultRows) < 2000 {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range raw {
			raw[i] = normalizeSQLValue(v)
		}
		resultRows = append(resultRows, raw)
	}
	return &engine.SQLResult{
		Columns:    cols,
		Rows:       resultRows,
		DurationMs: time.Since(started).Milliseconds(),
		CommandTag: fmt.Sprintf("SELECT %d", len(resultRows)),
		IsSelect:   true,
	}, rows.Err()
}

func scanSQLRowsToMaps(rows *sql.Rows, cols []engine.ColumnInfo, pks []string) ([]map[string]any, []map[string]any, error) {
	resRows := make([]map[string]any, 0)
	locators := make([]map[string]any, 0)
	colCount := len(cols)
	for rows.Next() {
		raw := make([]any, colCount)
		ptrs := make([]any, colCount)
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		m := make(map[string]any, colCount)
		for i, c := range cols {
			m[c.Name] = normalizeSQLValue(raw[i])
		}
		resRows = append(resRows, m)
		loc := make(map[string]any)
		for _, pk := range pks {
			loc[pk] = m[pk]
		}
		locators = append(locators, loc)
	}
	return resRows, locators, rows.Err()
}

func normalizeSQLValue(v any) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339Nano)
	default:
		return t
	}
}

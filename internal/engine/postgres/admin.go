package postgres

import (
	"context"
	"fmt"
	"strings"

	"pg-admin-pro/internal/engine"
)

func (d *Driver) ListDatabases(ctx context.Context) ([]engine.DatabaseInfo, error) {
	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT
			d.datname,
			pg_get_userbyid(d.datdba) AS owner,
			pg_encoding_to_char(d.encoding) AS encoding,
			COALESCE(d.datcollate, '') AS collation,
			CASE WHEN has_database_privilege(current_user, d.datname, 'CONNECT')
				THEN pg_database_size(d.datname) ELSE 0 END::bigint AS size_bytes,
			(SELECT count(*)::int FROM pg_stat_activity s WHERE s.datname = d.datname) AS active_conns,
			d.datallowconn,
			COALESCE((
				SELECT array_agg(DISTINCT r.rolname ORDER BY r.rolname)
				FROM (
					SELECT pg_get_userbyid(d.datdba) AS rolname
					UNION ALL
					SELECT pg_get_userbyid(acl.grantee) AS rolname
					FROM aclexplode(d.datacl) AS acl
					WHERE acl.grantee <> 0
				) r
				WHERE r.rolname IS NOT NULL AND r.rolname <> ''
			), '{}'::text[]) AS assigned_users
		FROM pg_database d
		WHERE NOT d.datistemplate
		ORDER BY d.datname
	`)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	defer rows.Close()

	var result []engine.DatabaseInfo
	for rows.Next() {
		var db engine.DatabaseInfo
		if err := rows.Scan(
			&db.Name,
			&db.Owner,
			&db.Encoding,
			&db.Collation,
			&db.SizeBytes,
			&db.ActiveConns,
			&db.AllowConnect,
			&db.AssignedUsers,
		); err != nil {
			return nil, fmt.Errorf("scan database: %w", err)
		}
		db.SizePretty = engine.HumanBytes(db.SizeBytes)
		if db.AssignedUsers == nil {
			db.AssignedUsers = []string{}
		}
		result = append(result, db)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []engine.DatabaseInfo{}
	}
	return result, nil
}

func (d *Driver) CreateDatabase(ctx context.Context, req engine.CreateDatabaseReq) error {
	name, err := validateName(req.Name)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("CREATE DATABASE ")
	b.WriteString(quoteIdent(name))

	if owner := strings.TrimSpace(req.Owner); owner != "" {
		if _, err := validateName(owner); err != nil {
			return fmt.Errorf("%w: invalid owner: %v", engine.ErrInvalidInput, err)
		}
		b.WriteString(" OWNER ")
		b.WriteString(quoteIdent(owner))
	}

	if enc := strings.ToUpper(strings.TrimSpace(req.Encoding)); enc != "" {
		if !isSafeEncoding(enc) {
			return fmt.Errorf("%w: unsupported encoding %q", engine.ErrInvalidInput, enc)
		}
		lit, err := quoteLiteral(enc)
		if err != nil {
			return err
		}
		b.WriteString(" ENCODING ")
		b.WriteString(lit)
	}

	_, err = pool.Exec(ctx, b.String())
	if err != nil {
		return fmt.Errorf("create database %q: %w", name, err)
	}
	return nil
}

func (d *Driver) DropDatabase(ctx context.Context, dbName string) error {
	name, err := validateName(dbName)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	if name == "postgres" || name == d.cfg.Database {
		return fmt.Errorf("%w: cannot drop active or primary maintenance database %q", engine.ErrInvalidInput, name)
	}

	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}

	d.dropCachedPool(name)

	_, _ = pool.Exec(ctx,
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`,
		name,
	)

	_, err = pool.Exec(ctx, "DROP DATABASE "+quoteIdent(name))
	if err != nil {
		return fmt.Errorf("drop database %q: %w", name, err)
	}
	return nil
}

func (d *Driver) ListUsers(ctx context.Context) ([]engine.UserInfo, error) {
	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, `
		SELECT
			r.rolname,
			r.rolsuper,
			r.rolcanlogin,
			r.rolcreatedb,
			r.rolcreaterole,
			r.rolconnlimit,
			CASE WHEN r.rolvaliduntil IS NOT NULL THEN to_char(r.rolvaliduntil AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') ELSE NULL END,
			COALESCE((
				SELECT array_agg(d.datname ORDER BY d.datname)
				FROM pg_database d
				WHERE d.datdba = r.oid AND NOT d.datistemplate
			), '{}'::text[])
		FROM pg_roles r
		WHERE r.rolname NOT LIKE 'pg\_%' ESCAPE '\'
		ORDER BY r.rolsuper DESC, r.rolcanlogin DESC, r.rolname
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []engine.UserInfo
	for rows.Next() {
		var u engine.UserInfo
		if err := rows.Scan(
			&u.Name,
			&u.IsSuperuser,
			&u.CanLogin,
			&u.CanCreateDB,
			&u.CanCreateRole,
			&u.ConnLimit,
			&u.ValidUntil,
			&u.OwnedDatabases,
		); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		if u.OwnedDatabases == nil {
			u.OwnedDatabases = []string{}
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if users == nil {
		users = []engine.UserInfo{}
	}
	return users, nil
}

func (d *Driver) CreateUser(ctx context.Context, req engine.CreateUserReq) error {
	username, err := validateName(req.Username)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	if strings.HasPrefix(username, "pg_") {
		return fmt.Errorf("%w: role names cannot start with pg_", engine.ErrInvalidInput)
	}

	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}

	opts := buildRoleOptions(
		&req.CanLogin,
		&req.IsSuperuser,
		&req.CanCreateDB,
		&req.CanCreateRole,
		&req.ConnLimit,
	)

	var b strings.Builder
	b.WriteString("CREATE ROLE ")
	b.WriteString(quoteIdent(username))
	b.WriteString(" WITH ")
	b.WriteString(strings.Join(opts, " "))

	if req.Password != "" {
		lit, err := quoteLiteral(req.Password)
		if err != nil {
			return err
		}
		b.WriteString(" PASSWORD ")
		b.WriteString(lit)
	}

	if _, err := pool.Exec(ctx, b.String()); err != nil {
		return fmt.Errorf("create user %q: %w", username, err)
	}
	return nil
}

func (d *Driver) UpdateUser(ctx context.Context, username string, req engine.UpdateUserReq) error {
	name, err := validateName(username)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}

	opts := buildRoleOptions(req.CanLogin, req.IsSuperuser, req.CanCreateDB, req.CanCreateRole, req.ConnLimit)
	if req.Password != nil && *req.Password != "" {
		lit, err := quoteLiteral(*req.Password)
		if err != nil {
			return err
		}
		opts = append(opts, "PASSWORD "+lit)
	}

	if len(opts) == 0 {
		return nil
	}

	sql := "ALTER ROLE " + quoteIdent(name) + " WITH " + strings.Join(opts, " ")
	if _, err := pool.Exec(ctx, sql); err != nil {
		return fmt.Errorf("update user %q: %w", name, err)
	}
	return nil
}

func (d *Driver) DropUser(ctx context.Context, username string) error {
	name, err := validateName(username)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	if name == d.cfg.Username {
		return fmt.Errorf("%w: cannot drop currently connected admin user %q", engine.ErrInvalidInput, name)
	}
	pool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, "DROP ROLE "+quoteIdent(name)); err != nil {
		return fmt.Errorf("drop user %q: %w", name, err)
	}
	return nil
}

func (d *Driver) ListDatabaseGrants(ctx context.Context, dbName string) ([]engine.DatabaseGrant, error) {
	name, err := validateName(dbName)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	maintPool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return nil, err
	}

	rows, err := maintPool.Query(ctx, `
		SELECT
			r.rolname,
			(d.datdba = r.oid) AS is_owner,
			has_database_privilege(r.oid, d.oid, 'CONNECT') AS can_connect,
			has_database_privilege(r.oid, d.oid, 'CREATE') AS can_create,
			has_database_privilege(r.oid, d.oid, 'TEMP') AS can_temp
		FROM pg_roles r
		CROSS JOIN pg_database d
		WHERE d.datname = $1
		  AND r.rolname NOT LIKE 'pg\_%' ESCAPE '\'
		  AND r.rolcanlogin
		ORDER BY (d.datdba = r.oid) DESC, has_database_privilege(r.oid, d.oid, 'CONNECT') DESC, r.rolname
	`, name)
	if err != nil {
		return nil, fmt.Errorf("list database grants: %w", err)
	}
	defer rows.Close()

	var grants []engine.DatabaseGrant
	for rows.Next() {
		var g engine.DatabaseGrant
		if err := rows.Scan(&g.Username, &g.IsOwner, &g.CanConnect, &g.CanCreate, &g.CanTemp); err != nil {
			return nil, err
		}
		if g.IsOwner {
			g.SchemaAccess = "owner"
		} else if g.CanCreate && g.CanConnect {
			g.SchemaAccess = "admin"
		} else if g.CanConnect {
			g.SchemaAccess = "connect"
		} else {
			g.SchemaAccess = "none"
		}
		grants = append(grants, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Enrich schema & table privilege level from inside the target database when accessible.
	if targetPool, err := d.getPool(ctx, name); err == nil {
		schemaMap := make(map[string]string)
		sRows, sErr := targetPool.Query(ctx, `
			SELECT
				r.rolname,
				CASE
					WHEN r.rolsuper THEN 'superuser'
					WHEN EXISTS (SELECT 1 FROM pg_namespace n WHERE n.nspname = 'public' AND has_schema_privilege(r.oid, n.oid, 'CREATE'))
					  AND EXISTS (SELECT 1 FROM pg_namespace n WHERE n.nspname = 'public' AND has_schema_privilege(r.oid, n.oid, 'USAGE'))
						THEN 'admin'
					WHEN EXISTS (SELECT 1 FROM pg_namespace n WHERE n.nspname = 'public' AND has_schema_privilege(r.oid, n.oid, 'USAGE'))
					  AND EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind = 'r' AND has_table_privilege(r.oid, c.oid, 'INSERT'))
						THEN 'read_write'
					WHEN EXISTS (SELECT 1 FROM pg_namespace n WHERE n.nspname = 'public' AND has_schema_privilege(r.oid, n.oid, 'USAGE'))
					  AND (
						EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind = 'r' AND has_table_privilege(r.oid, c.oid, 'SELECT'))
						OR NOT EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind = 'r')
					  )
						THEN 'read_only'
					ELSE ''
				END AS access_level
			FROM pg_roles r
			WHERE r.rolname NOT LIKE 'pg\_%' ESCAPE '\' AND r.rolcanlogin
		`)
		if sErr == nil {
			defer sRows.Close()
			for sRows.Next() {
				var uname, lvl string
				if err := sRows.Scan(&uname, &lvl); err == nil && lvl != "" {
					schemaMap[uname] = lvl
				}
			}
		}

		for i := range grants {
			if grants[i].IsOwner {
				grants[i].SchemaAccess = "owner"
				continue
			}
			if !grants[i].CanConnect {
				grants[i].SchemaAccess = "none"
				continue
			}
			if lvl, ok := schemaMap[grants[i].Username]; ok && lvl != "" {
				if lvl == "superuser" {
					grants[i].SchemaAccess = "admin"
				} else {
					grants[i].SchemaAccess = lvl
				}
			}
		}
	}

	if grants == nil {
		grants = []engine.DatabaseGrant{}
	}
	return grants, nil
}

func (d *Driver) AssignUserToDatabase(ctx context.Context, dbName string, req engine.AssignUserReq) error {
	db, err := validateName(dbName)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	user, err := validateName(req.Username)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	schema := strings.TrimSpace(req.Schema)
	if schema == "" {
		schema = "public"
	}
	if _, err := validateName(schema); err != nil {
		return fmt.Errorf("%w: invalid schema: %v", engine.ErrInvalidInput, err)
	}

	maintPool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}

	qDB := quoteIdent(db)
	qUser := quoteIdent(user)
	qSchema := quoteIdent(schema)

	// Step 1: Apply cluster/database-level privileges first.
	switch req.Preset {
	case "owner":
		if _, err := maintPool.Exec(ctx, "ALTER DATABASE "+qDB+" OWNER TO "+qUser); err != nil {
			return fmt.Errorf("set database owner: %w", err)
		}
		if _, err := maintPool.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+qDB+" TO "+qUser); err != nil {
			return fmt.Errorf("grant db privileges: %w", err)
		}
	case "admin":
		if _, err := maintPool.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+qDB+" TO "+qUser); err != nil {
			return fmt.Errorf("grant db privileges: %w", err)
		}
	case "read_write":
		_, _ = maintPool.Exec(ctx, "REVOKE CREATE ON DATABASE "+qDB+" FROM "+qUser)
		if _, err := maintPool.Exec(ctx, "GRANT CONNECT, TEMP ON DATABASE "+qDB+" TO "+qUser); err != nil {
			return fmt.Errorf("grant connect/temp: %w", err)
		}
	case "read_only":
		_, _ = maintPool.Exec(ctx, "REVOKE CREATE, TEMP ON DATABASE "+qDB+" FROM "+qUser)
		if _, err := maintPool.Exec(ctx, "GRANT CONNECT ON DATABASE "+qDB+" TO "+qUser); err != nil {
			return fmt.Errorf("grant connect: %w", err)
		}
	case "connect_only":
		_, _ = maintPool.Exec(ctx, "REVOKE CREATE, TEMP ON DATABASE "+qDB+" FROM "+qUser)
		if _, err := maintPool.Exec(ctx, "GRANT CONNECT ON DATABASE "+qDB+" TO "+qUser); err != nil {
			return fmt.Errorf("grant connect: %w", err)
		}
	default:
		return fmt.Errorf("%w: unsupported preset %q", engine.ErrInvalidInput, req.Preset)
	}

	// Step 2: Apply schema, existing tables/sequences, and default privileges inside target DB.
	targetPool, err := d.getPool(ctx, db)
	if err != nil {
		return fmt.Errorf("open target db %q for schema grants: %w", db, err)
	}

	tx, err := targetPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var stmts []string
	switch req.Preset {
	case "owner":
		stmts = append(stmts,
			"ALTER SCHEMA "+qSchema+" OWNER TO "+qUser,
			"GRANT ALL PRIVILEGES ON SCHEMA "+qSchema+" TO "+qUser,
			"GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA "+qSchema+" TO "+qUser,
			"GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA "+qSchema+" TO "+qUser,
			"GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA "+qSchema+" TO "+qUser,
		)
	case "admin":
		stmts = append(stmts,
			"GRANT ALL PRIVILEGES ON SCHEMA "+qSchema+" TO "+qUser,
			"GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA "+qSchema+" TO "+qUser,
			"GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA "+qSchema+" TO "+qUser,
			"GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA "+qSchema+" TO "+qUser,
		)
		if req.ApplyDefaultPrivs {
			stmts = append(stmts,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA "+qSchema+" GRANT ALL PRIVILEGES ON TABLES TO "+qUser,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA "+qSchema+" GRANT ALL PRIVILEGES ON SEQUENCES TO "+qUser,
			)
		}
	case "read_write":
		stmts = append(stmts,
			"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA "+qSchema+" FROM "+qUser,
			"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA "+qSchema+" FROM "+qUser,
			"REVOKE CREATE ON SCHEMA "+qSchema+" FROM "+qUser,
			"GRANT USAGE ON SCHEMA "+qSchema+" TO "+qUser,
			"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA "+qSchema+" TO "+qUser,
			"GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA "+qSchema+" TO "+qUser,
		)
		if req.ApplyDefaultPrivs {
			stmts = append(stmts,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA "+qSchema+" REVOKE ALL ON TABLES FROM "+qUser,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA "+qSchema+" GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO "+qUser,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA "+qSchema+" GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO "+qUser,
			)
		}
	case "read_only":
		stmts = append(stmts,
			"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA "+qSchema+" FROM "+qUser,
			"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA "+qSchema+" FROM "+qUser,
			"REVOKE CREATE ON SCHEMA "+qSchema+" FROM "+qUser,
			"GRANT USAGE ON SCHEMA "+qSchema+" TO "+qUser,
			"GRANT SELECT ON ALL TABLES IN SCHEMA "+qSchema+" TO "+qUser,
			"GRANT SELECT ON ALL SEQUENCES IN SCHEMA "+qSchema+" TO "+qUser,
		)
		if req.ApplyDefaultPrivs {
			stmts = append(stmts,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA "+qSchema+" REVOKE ALL ON TABLES FROM "+qUser,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA "+qSchema+" GRANT SELECT ON TABLES TO "+qUser,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA "+qSchema+" GRANT SELECT ON SEQUENCES TO "+qUser,
			)
		}
	case "connect_only":
		stmts = append(stmts,
			"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA "+qSchema+" FROM "+qUser,
			"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA "+qSchema+" FROM "+qUser,
			"REVOKE ALL PRIVILEGES ON SCHEMA "+qSchema+" FROM "+qUser,
		)
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply grant (%s): %w", stmt, err)
		}
	}

	return tx.Commit(ctx)
}

func (d *Driver) RevokeUserFromDatabase(ctx context.Context, dbName, username string) error {
	db, err := validateName(dbName)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}
	user, err := validateName(username)
	if err != nil {
		return fmt.Errorf("%w: %v", engine.ErrInvalidInput, err)
	}

	maintPool, err := d.getPool(ctx, d.cfg.Database)
	if err != nil {
		return err
	}

	var owner string
	if err := maintPool.QueryRow(ctx, `SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = $1`, db).Scan(&owner); err == nil {
		if owner == user {
			return fmt.Errorf("%w: %q is the owner of database %q; transfer ownership to another user first", engine.ErrInvalidInput, user, db)
		}
	}

	qUser := quoteIdent(user)
	qDB := quoteIdent(db)

	if targetPool, err := d.getPool(ctx, db); err == nil {
		tx, txErr := targetPool.Begin(ctx)
		if txErr == nil {
			stmts := []string{
				"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM " + qUser,
				"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM " + qUser,
				"REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public FROM " + qUser,
				"REVOKE ALL PRIVILEGES ON SCHEMA public FROM " + qUser,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES FROM " + qUser,
				"ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON SEQUENCES FROM " + qUser,
			}
			for _, s := range stmts {
				_, _ = tx.Exec(ctx, s)
			}
			_ = tx.Commit(ctx)
		}
	}

	if _, err := maintPool.Exec(ctx, "REVOKE ALL PRIVILEGES ON DATABASE "+qDB+" FROM "+qUser); err != nil {
		return fmt.Errorf("revoke database privileges: %w", err)
	}
	return nil
}

func buildRoleOptions(canLogin, isSuper, canCreateDB, canCreateRole *bool, connLimit *int) []string {
	var opts []string
	if canLogin != nil {
		if *canLogin {
			opts = append(opts, "LOGIN")
		} else {
			opts = append(opts, "NOLOGIN")
		}
	}
	if isSuper != nil {
		if *isSuper {
			opts = append(opts, "SUPERUSER")
		} else {
			opts = append(opts, "NOSUPERUSER")
		}
	}
	if canCreateDB != nil {
		if *canCreateDB {
			opts = append(opts, "CREATEDB")
		} else {
			opts = append(opts, "NOCREATEDB")
		}
	}
	if canCreateRole != nil {
		if *canCreateRole {
			opts = append(opts, "CREATEROLE")
		} else {
			opts = append(opts, "NOCREATEROLE")
		}
	}
	if connLimit != nil {
		opts = append(opts, fmt.Sprintf("CONNECTION LIMIT %d", *connLimit))
	}
	return opts
}

func isSafeEncoding(enc string) bool {
	switch enc {
	case "UTF8", "SQL_ASCII", "LATIN1", "LATIN2", "WIN1252":
		return true
	default:
		return false
	}
}

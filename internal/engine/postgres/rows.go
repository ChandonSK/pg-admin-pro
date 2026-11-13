package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"pg-admin-pro/internal/engine"
)

func buildRowQueryClauses(
	structure *engine.TableStructure,
	req engine.QueryRowsReq,
) (colMap map[string]engine.ColumnInfo, selectCols []string, whereSQL string, orderSQL string, args []any, err error) {
	colMap = make(map[string]engine.ColumnInfo, len(structure.Columns))
	selectCols = make([]string, 0, len(structure.Columns)+1)
	for _, c := range structure.Columns {
		colMap[c.Name] = c
		selectCols = append(selectCols, quoteIdent(c.Name))
	}

	var whereParts []string
	if search := strings.TrimSpace(req.Search); search != "" && len(structure.Columns) > 0 {
		args = append(args, "%"+search+"%")
		paramIdx := len(args)
		var orParts []string
		for _, c := range structure.Columns {
			orParts = append(orParts, fmt.Sprintf("%s::text ILIKE $%d", quoteIdent(c.Name), paramIdx))
		}
		whereParts = append(whereParts, "("+strings.Join(orParts, " OR ")+")")
	}

	for _, f := range req.Filters {
		col, ok := colMap[f.Column]
		if !ok {
			return nil, nil, "", "", nil, fmt.Errorf("%w: unknown filter column %q", engine.ErrInvalidInput, f.Column)
		}
		qCol := quoteIdent(col.Name)
		switch f.Operator {
		case "is_null":
			whereParts = append(whereParts, qCol+" IS NULL")
		case "is_not_null":
			whereParts = append(whereParts, qCol+" IS NOT NULL")
		case "ilike", "like":
			val := f.Value
			if !strings.ContainsAny(val, "%_") {
				val = "%" + val + "%"
			}
			args = append(args, val)
			kw := "ILIKE"
			if f.Operator == "like" {
				kw = "LIKE"
			}
			whereParts = append(whereParts, fmt.Sprintf("%s::text %s $%d", qCol, kw, len(args)))
		case "=", "!=", ">", ">=", "<", "<=":
			args = append(args, f.Value)
			op := f.Operator
			if op == "!=" {
				op = "<>"
			}
			whereParts = append(whereParts, fmt.Sprintf("%s %s $%d::%s", qCol, op, len(args), col.FormatType))
		default:
			return nil, nil, "", "", nil, fmt.Errorf("%w: unsupported operator %q", engine.ErrInvalidInput, f.Operator)
		}
	}

	if len(whereParts) > 0 {
		whereSQL = " WHERE " + strings.Join(whereParts, " AND ")
	}

	if sortCol := strings.TrimSpace(req.SortBy); sortCol != "" {
		if _, ok := colMap[sortCol]; !ok {
			return nil, nil, "", "", nil, fmt.Errorf("%w: unknown sort column %q", engine.ErrInvalidInput, sortCol)
		}
		dir := "ASC"
		if strings.EqualFold(req.SortDir, "desc") {
			dir = "DESC"
		}
		orderSQL = " ORDER BY " + quoteIdent(sortCol) + " " + dir
	} else if len(structure.PrimaryKeys) > 0 {
		orderSQL = " ORDER BY " + quoteIdent(structure.PrimaryKeys[0]) + " ASC"
	} else if structure.UsesRowIDFallback {
		orderSQL = " ORDER BY ctid ASC"
	}
	return colMap, selectCols, whereSQL, orderSQL, args, nil
}

func (d *Driver) QueryRows(ctx context.Context, dbName string, req engine.QueryRowsReq) (*engine.QueryRowsResp, error) {
	started := time.Now()
	if strings.TrimSpace(req.Schema) == "" {
		req.Schema = "public"
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 500 {
		req.PageSize = 50
	}

	structure, err := d.GetTableStructure(ctx, dbName, req.Schema, req.Table)
	if err != nil {
		return nil, err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	_, selectCols, whereSQL, orderSQL, args, err := buildRowQueryClauses(structure, req)
	if err != nil {
		return nil, err
	}
	if structure.UsesRowIDFallback {
		selectCols = append(selectCols, "ctid::text")
	}

	qTable := quoteIdent(structure.Schema, structure.Table)

	var totalCount int64
	countSQL := "SELECT count(*)::bigint FROM " + qTable + whereSQL
	if err := pool.QueryRow(ctx, countSQL, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count rows: %w", err)
	}

	offset := (req.Page - 1) * req.PageSize
	dataArgs := append(append([]any{}, args...), req.PageSize, offset)
	dataSQL := fmt.Sprintf(
		"SELECT %s FROM %s%s%s LIMIT $%d OFFSET $%d",
		strings.Join(selectCols, ", "),
		qTable,
		whereSQL,
		orderSQL,
		len(args)+1,
		len(args)+2,
	)

	rows, err := pool.Query(ctx, dataSQL, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("query rows: %w", err)
	}
	defer rows.Close()

	resultRows := make([]map[string]any, 0)
	rowLocators := make([]map[string]any, 0)
	colCount := len(structure.Columns)

	for rows.Next() {
		rawVals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		rowMap := make(map[string]any, colCount)
		for i, col := range structure.Columns {
			rowMap[col.Name] = normalizeValue(rawVals[i])
		}
		resultRows = append(resultRows, rowMap)

		loc := make(map[string]any)
		if structure.UsesRowIDFallback {
			loc["__ctid"] = normalizeValue(rawVals[colCount])
		} else {
			for _, pk := range structure.PrimaryKeys {
				loc[pk] = rowMap[pk]
			}
		}
		rowLocators = append(rowLocators, loc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &engine.QueryRowsResp{
		Columns:           structure.Columns,
		PrimaryKeys:       structure.PrimaryKeys,
		UsesRowIDFallback: structure.UsesRowIDFallback,
		Rows:              resultRows,
		RowLocators:       rowLocators,
		TotalCount:        totalCount,
		Page:              req.Page,
		PageSize:          req.PageSize,
		DurationMs:        time.Since(started).Milliseconds(),
	}, nil
}

func (d *Driver) StreamRows(
	ctx context.Context,
	dbName string,
	req engine.QueryRowsReq,
	allRows bool,
	onHeader func([]engine.ColumnInfo) error,
	onRow func([]any) error,
) error {
	if strings.TrimSpace(req.Schema) == "" {
		req.Schema = "public"
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 5000 {
		req.PageSize = 50
	}

	structure, err := d.GetTableStructure(ctx, dbName, req.Schema, req.Table)
	if err != nil {
		return err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return err
	}

	_, selectCols, whereSQL, orderSQL, args, err := buildRowQueryClauses(structure, req)
	if err != nil {
		return err
	}
	if err := onHeader(structure.Columns); err != nil {
		return err
	}
	if len(selectCols) == 0 {
		return nil
	}

	qTable := quoteIdent(structure.Schema, structure.Table)
	var dataSQL string
	if allRows {
		dataSQL = fmt.Sprintf("SELECT %s FROM %s%s%s", strings.Join(selectCols, ", "), qTable, whereSQL, orderSQL)
	} else {
		offset := (req.Page - 1) * req.PageSize
		args = append(args, req.PageSize, offset)
		dataSQL = fmt.Sprintf(
			"SELECT %s FROM %s%s%s LIMIT $%d OFFSET $%d",
			strings.Join(selectCols, ", "),
			qTable,
			whereSQL,
			orderSQL,
			len(args)-1,
			len(args),
		)
	}

	rows, err := pool.Query(ctx, dataSQL, args...)
	if err != nil {
		return fmt.Errorf("stream rows: %w", err)
	}
	defer rows.Close()

	colCount := len(structure.Columns)
	for rows.Next() {
		rawVals, err := rows.Values()
		if err != nil {
			return err
		}
		out := make([]any, colCount)
		for i := 0; i < colCount && i < len(rawVals); i++ {
			out[i] = normalizeValue(rawVals[i])
		}
		if err := onRow(out); err != nil {
			return err
		}
	}
	return rows.Err()
}

func buildInsertSQL(
	structure *engine.TableStructure,
	colMap map[string]engine.ColumnInfo,
	values map[string]any,
	returning bool,
) (string, []any, error) {
	var colNames []string
	var placeholders []string
	var args []any

	for _, col := range structure.Columns {
		rawVal, provided := values[col.Name]
		if !provided || col.IsGenerated {
			continue
		}
		paramVal, err := toParamString(rawVal)
		if err != nil {
			return "", nil, fmt.Errorf("%w: column %q: %v", engine.ErrInvalidInput, col.Name, err)
		}
		colNames = append(colNames, quoteIdent(col.Name))
		if paramVal == nil {
			placeholders = append(placeholders, "NULL")
		} else {
			args = append(args, paramVal)
			placeholders = append(placeholders, fmt.Sprintf("$%d::%s", len(args), col.FormatType))
		}
	}

	for k := range values {
		if _, ok := colMap[k]; !ok {
			return "", nil, fmt.Errorf("%w: unknown column %q", engine.ErrInvalidInput, k)
		}
	}

	qTable := quoteIdent(structure.Schema, structure.Table)
	var sql string
	if len(colNames) == 0 {
		sql = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", qTable)
	} else {
		sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", qTable, strings.Join(colNames, ", "), strings.Join(placeholders, ", "))
	}
	if returning {
		sql += " RETURNING *"
	}
	return sql, args, nil
}

func (d *Driver) InsertRow(ctx context.Context, dbName, schema, table string, values map[string]any) (map[string]any, error) {
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}
	structure, err := d.GetTableStructure(ctx, dbName, schema, table)
	if err != nil {
		return nil, err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	colMap := make(map[string]engine.ColumnInfo, len(structure.Columns))
	for _, c := range structure.Columns {
		colMap[c.Name] = c
	}

	sql, args, err := buildInsertSQL(structure, colMap, values, true)
	if err != nil {
		return nil, err
	}
	return scanSingleReturnedRow(ctx, pool, sql, args...)
}

func (d *Driver) ImportRows(ctx context.Context, dbName, schema, table string, req engine.ImportRowsReq) (*engine.ImportResult, error) {
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}
	if len(req.Rows) == 0 {
		return nil, fmt.Errorf("%w: no rows supplied for import", engine.ErrInvalidInput)
	}
	started := time.Now()
	structure, err := d.GetTableStructure(ctx, dbName, schema, table)
	if err != nil {
		return nil, err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	colMap := make(map[string]engine.ColumnInfo, len(structure.Columns))
	for _, c := range structure.Columns {
		colMap[c.Name] = c
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	res := &engine.ImportResult{}
	for idx, row := range req.Rows {
		sql, args, buildErr := buildInsertSQL(structure, colMap, row, false)
		if buildErr != nil {
			if !req.ContinueOnError {
				return nil, fmt.Errorf("row %d: %w", idx+1, buildErr)
			}
			res.Failed++
			if len(res.Errors) < 100 {
				res.Errors = append(res.Errors, engine.ImportRowError{RowIndex: idx + 1, Error: buildErr.Error()})
			}
			continue
		}

		if req.ContinueOnError {
			if _, spErr := tx.Exec(ctx, "SAVEPOINT imp_row"); spErr != nil {
				return nil, spErr
			}
		}
		if _, execErr := tx.Exec(ctx, sql, args...); execErr != nil {
			if !req.ContinueOnError {
				return nil, fmt.Errorf("row %d: %w", idx+1, execErr)
			}
			_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT imp_row")
			res.Failed++
			if len(res.Errors) < 100 {
				res.Errors = append(res.Errors, engine.ImportRowError{RowIndex: idx + 1, Error: execErr.Error()})
			}
			continue
		}
		if req.ContinueOnError {
			_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT imp_row")
		}
		res.Inserted++
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	res.DurationMs = time.Since(started).Milliseconds()
	return res, nil
}

func (d *Driver) UpdateRow(ctx context.Context, dbName, schema, table string, locator, updates map[string]any) (map[string]any, error) {
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("%w: no fields provided to update", engine.ErrInvalidInput)
	}
	structure, err := d.GetTableStructure(ctx, dbName, schema, table)
	if err != nil {
		return nil, err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	colMap := make(map[string]engine.ColumnInfo, len(structure.Columns))
	for _, c := range structure.Columns {
		colMap[c.Name] = c
	}

	var setParts []string
	var args []any
	for _, col := range structure.Columns {
		rawVal, provided := updates[col.Name]
		if !provided || col.IsGenerated {
			continue
		}
		paramVal, err := toParamString(rawVal)
		if err != nil {
			return nil, fmt.Errorf("%w: column %q: %v", engine.ErrInvalidInput, col.Name, err)
		}
		if paramVal == nil {
			setParts = append(setParts, quoteIdent(col.Name)+" = NULL")
		} else {
			args = append(args, paramVal)
			setParts = append(setParts, fmt.Sprintf("%s = $%d::%s", quoteIdent(col.Name), len(args), col.FormatType))
		}
	}
	for k := range updates {
		if _, ok := colMap[k]; !ok {
			return nil, fmt.Errorf("%w: unknown column %q", engine.ErrInvalidInput, k)
		}
	}
	if len(setParts) == 0 {
		return nil, fmt.Errorf("%w: no updatable columns provided", engine.ErrInvalidInput)
	}

	whereClause, whereArgs, err := buildLocatorWhere(structure, colMap, locator, len(args)+1)
	if err != nil {
		return nil, err
	}
	args = append(args, whereArgs...)

	sql := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s RETURNING *",
		quoteIdent(structure.Schema, structure.Table),
		strings.Join(setParts, ", "),
		whereClause,
	)
	return scanSingleReturnedRow(ctx, pool, sql, args...)
}

func (d *Driver) DeleteRows(ctx context.Context, dbName, schema, table string, locators []map[string]any) (int64, error) {
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}
	if len(locators) == 0 {
		return 0, nil
	}
	structure, err := d.GetTableStructure(ctx, dbName, schema, table)
	if err != nil {
		return 0, err
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return 0, err
	}

	colMap := make(map[string]engine.ColumnInfo, len(structure.Columns))
	for _, c := range structure.Columns {
		colMap[c.Name] = c
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	qTable := quoteIdent(structure.Schema, structure.Table)
	var deleted int64
	for _, loc := range locators {
		whereClause, whereArgs, err := buildLocatorWhere(structure, colMap, loc, 1)
		if err != nil {
			return 0, err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM "+qTable+" WHERE "+whereClause, whereArgs...)
		if err != nil {
			return 0, fmt.Errorf("delete row: %w", err)
		}
		deleted += tag.RowsAffected()
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (d *Driver) ExecuteSQL(ctx context.Context, dbName, sqlText string) (*engine.SQLResult, error) {
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return nil, fmt.Errorf("%w: SQL query cannot be empty", engine.ErrInvalidInput)
	}
	pool, err := d.getPool(ctx, dbName)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	rows, err := pool.Query(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	colNames := make([]string, len(fields))
	for i, f := range fields {
		colNames[i] = f.Name
	}

	resultRows := make([][]any, 0)
	const maxDisplayRows = 2000
	for rows.Next() {
		if len(resultRows) >= maxDisplayRows {
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		rowSlice := make([]any, len(vals))
		for i, v := range vals {
			rowSlice[i] = normalizeValue(v)
		}
		resultRows = append(resultRows, rowSlice)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tag := rows.CommandTag()
	return &engine.SQLResult{
		Columns:      colNames,
		Rows:         resultRows,
		RowsAffected: tag.RowsAffected(),
		DurationMs:   time.Since(started).Milliseconds(),
		CommandTag:   tag.String(),
		IsSelect:     len(fields) > 0,
	}, nil
}

func buildLocatorWhere(
	structure *engine.TableStructure,
	colMap map[string]engine.ColumnInfo,
	locator map[string]any,
	startParam int,
) (string, []any, error) {
	if structure.UsesRowIDFallback {
		ctidVal, ok := locator["__ctid"]
		if !ok || ctidVal == nil {
			return "", nil, fmt.Errorf("%w: missing __ctid row locator", engine.ErrInvalidInput)
		}
		s, err := toParamString(ctidVal)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("ctid = $%d::tid", startParam), []any{s}, nil
	}

	if len(structure.PrimaryKeys) == 0 {
		return "", nil, fmt.Errorf("%w: table %s.%s has no primary key or ctid locator", engine.ErrInvalidInput, structure.Schema, structure.Table)
	}

	var parts []string
	var args []any
	for _, pk := range structure.PrimaryKeys {
		rawVal, ok := locator[pk]
		if !ok || rawVal == nil {
			return "", nil, fmt.Errorf("%w: missing primary key %q in locator", engine.ErrInvalidInput, pk)
		}
		pVal, err := toParamString(rawVal)
		if err != nil {
			return "", nil, err
		}
		col := colMap[pk]
		idx := startParam + len(args)
		args = append(args, pVal)
		parts = append(parts, fmt.Sprintf("%s = $%d::%s", quoteIdent(pk), idx, col.FormatType))
	}
	return strings.Join(parts, " AND "), args, nil
}

func scanSingleReturnedRow(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) (map[string]any, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("no matching row affected")
	}

	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	res := make(map[string]any, len(fields))
	for i, f := range fields {
		res[f.Name] = normalizeValue(vals[i])
	}
	return res, rows.Err()
}

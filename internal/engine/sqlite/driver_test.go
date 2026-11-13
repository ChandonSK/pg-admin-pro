package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"pg-admin-pro/internal/backup"
	"pg-admin-pro/internal/config"
	"pg-admin-pro/internal/engine"
	"pg-admin-pro/internal/engine/sqlite"
)

func TestSQLiteEndToEndCRUDAndBackup(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_erp.db")

	drv, err := sqlite.New(ctx, engine.ConnectionConfig{
		Engine:   engine.EngineSQLite,
		Database: dbPath,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer drv.Close()

	ov, err := drv.Overview(ctx)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.Engine != engine.EngineSQLite || ov.TotalDatabases != 1 {
		t.Fatalf("unexpected overview: %+v", ov)
	}

	err = drv.CreateTable(ctx, "", engine.CreateTableReq{
		Schema: "main",
		Table:  "students",
		Columns: []engine.CreateColumnDef{
			{Name: "id", Type: "INTEGER", Nullable: false, PrimaryKey: true},
			{Name: "name", Type: "TEXT", Nullable: false},
			{Name: "roll", Type: "INTEGER", Nullable: false},
		},
	})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	tables, err := drv.ListTables(ctx, "", "main")
	if err != nil || len(tables) != 1 || tables[0].Name != "students" {
		t.Fatalf("unexpected tables: %+v, err=%v", tables, err)
	}

	// Insert 2 rows
	if _, err := drv.InsertRow(ctx, "", "main", "students", map[string]any{"id": 1, "name": "Ayesha", "roll": 101}); err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	if _, err := drv.InsertRow(ctx, "", "main", "students", map[string]any{"id": 2, "name": "Rahim", "roll": 102}); err != nil {
		t.Fatalf("insert 2: %v", err)
	}

	// Query with search
	qResp, err := drv.QueryRows(ctx, "", engine.QueryRowsReq{
		Schema:   "main",
		Table:    "students",
		Page:     1,
		PageSize: 10,
		Search:   "Ayesha",
	})
	if err != nil {
		t.Fatalf("query rows: %v", err)
	}
	if qResp.TotalCount != 1 || len(qResp.Rows) != 1 || qResp.Rows[0]["name"] != "Ayesha" {
		t.Fatalf("unexpected search result: %+v", qResp)
	}

	// Update row via locator
	if _, err := drv.UpdateRow(ctx, "", "main", "students", qResp.RowLocators[0], map[string]any{"name": "Ayesha Siddika"}); err != nil {
		t.Fatalf("update row: %v", err)
	}

	// Verify update via SQL Runner
	sqlRes, err := drv.ExecuteSQL(ctx, "", "SELECT name FROM students WHERE id = 1")
	if err != nil || !sqlRes.IsSelect || len(sqlRes.Rows) != 1 || sqlRes.Rows[0][0] != "Ayesha Siddika" {
		t.Fatalf("unexpected sql result: %+v, err=%v", sqlRes, err)
	}

	// Backup using SQLite VACUUM INTO
	backupDir := filepath.Join(tmpDir, "backups")
	cfg := &config.Config{BackupDir: backupDir}
	mgr := backup.NewManager(cfg)
	bRes, err := mgr.CreateBackup(ctx, drv, "test_erp", 6)
	if err != nil {
		t.Fatalf("sqlite backup failed: %v", err)
	}
	if bRes.File.SizeBytes == 0 {
		t.Fatalf("expected non-empty backup file")
	}

	// Delete row
	deleted, err := drv.DeleteRows(ctx, "", "main", "students", []map[string]any{qResp.RowLocators[0]})
	if err != nil || deleted != 1 {
		t.Fatalf("delete rows: deleted=%d err=%v", deleted, err)
	}
}

func TestSQLiteDDLExportImportExplainObjects(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "extended.db")

	drv, err := sqlite.New(ctx, engine.ConnectionConfig{
		Engine:   engine.EngineSQLite,
		Database: dbPath,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer drv.Close()

	if err := drv.CreateTable(ctx, "", engine.CreateTableReq{
		Schema: "main",
		Table:  "enrollments",
		Columns: []engine.CreateColumnDef{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "student_name", Type: "TEXT", Nullable: false},
		},
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Add column + rename column + create index
	if err := drv.AddColumn(ctx, "", engine.AddColumnReq{
		Schema: "main", Table: "enrollments", Name: "grade", Type: "TEXT", Nullable: true, Default: "'A'",
	}); err != nil {
		t.Fatalf("add column: %v", err)
	}
	if err := drv.AlterColumn(ctx, "", engine.AlterColumnReq{
		Schema: "main", Table: "enrollments", Column: "grade", NewName: "letter_grade",
	}); err != nil {
		t.Fatalf("rename column: %v", err)
	}
	if err := drv.CreateIndex(ctx, "", engine.CreateIndexReq{
		Schema: "main", Table: "enrollments", Name: "idx_enroll_student", Columns: []string{"student_name"},
	}); err != nil {
		t.Fatalf("create index: %v", err)
	}

	// Import rows (1 duplicate conflict test with continueOnError=true)
	impRes, err := drv.ImportRows(ctx, "", "main", "enrollments", engine.ImportRowsReq{
		Rows: []map[string]any{
			{"id": 1, "student_name": "Nadia", "letter_grade": "A+"},
			{"id": 2, "student_name": "Karim", "letter_grade": "A"},
			{"id": 1, "student_name": "Dup", "letter_grade": "B"},
		},
		ContinueOnError: true,
	})
	if err != nil || impRes.Inserted != 2 || impRes.Failed != 1 {
		t.Fatalf("unexpected import result: %+v, err=%v", impRes, err)
	}

	// StreamRows
	var streamedCount int
	err = drv.StreamRows(ctx, "", engine.QueryRowsReq{Schema: "main", Table: "enrollments"}, true,
		func(cols []engine.ColumnInfo) error {
			if len(cols) != 3 {
				t.Fatalf("expected 3 columns, got %d", len(cols))
			}
			return nil
		},
		func(vals []any) error {
			streamedCount++
			return nil
		},
	)
	if err != nil || streamedCount != 2 {
		t.Fatalf("stream rows: count=%d err=%v", streamedCount, err)
	}

	// ExplainSQL + Autocomplete
	exp, err := drv.ExplainSQL(ctx, "", "SELECT * FROM enrollments WHERE student_name = 'Nadia'", false)
	if err != nil || len(exp.PlanLines) == 0 {
		t.Fatalf("explain failed: %+v, err=%v", exp, err)
	}
	cat, err := drv.ListAutocompleteCatalog(ctx, "")
	if err != nil || len(cat) != 1 || len(cat[0].Columns) != 3 {
		t.Fatalf("autocomplete mismatch: %+v, err=%v", cat, err)
	}

	// Drop index + Truncate
	if err := drv.DropIndex(ctx, "", "main", "enrollments", "idx_enroll_student"); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	if err := drv.TruncateTable(ctx, "", "main", "enrollments", false); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	qAfter, err := drv.QueryRows(ctx, "", engine.QueryRowsReq{Schema: "main", Table: "enrollments", Page: 1, PageSize: 10})
	if err != nil || qAfter.TotalCount != 0 {
		t.Fatalf("expected 0 rows after truncate, got %d, err=%v", qAfter.TotalCount, err)
	}
}

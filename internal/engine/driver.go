package engine

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrUnsupported = errors.New("operation is not supported by this database engine")
	ErrInvalidInput = errors.New("invalid input")
)

// Driver defines the unified database administration contract implemented by each engine.
type Driver interface {
	Kind() EngineKind
	Capabilities() Capabilities
	Config() PublicConnectionInfo
	Close() error

	// Cluster / Server Overview
	Ping(ctx context.Context) error
	Overview(ctx context.Context) (*ServerOverview, error)

	// Database Management
	ListDatabases(ctx context.Context) ([]DatabaseInfo, error)
	CreateDatabase(ctx context.Context, req CreateDatabaseReq) error
	DropDatabase(ctx context.Context, dbName string) error

	// User / Role Management
	ListUsers(ctx context.Context) ([]UserInfo, error)
	CreateUser(ctx context.Context, req CreateUserReq) error
	UpdateUser(ctx context.Context, username string, req UpdateUserReq) error
	DropUser(ctx context.Context, username string) error

	// Database-User Assignment & Privileges
	ListDatabaseGrants(ctx context.Context, dbName string) ([]DatabaseGrant, error)
	AssignUserToDatabase(ctx context.Context, dbName string, req AssignUserReq) error
	RevokeUserFromDatabase(ctx context.Context, dbName, username string) error

	// Schema & Table Inspection & DDL
	ListSchemas(ctx context.Context, dbName string, includeSystem bool) ([]SchemaInfo, error)
	ListTables(ctx context.Context, dbName, schema string) ([]TableInfo, error)
	GetTableStructure(ctx context.Context, dbName, schema, table string) (*TableStructure, error)
	CreateTable(ctx context.Context, dbName string, req CreateTableReq) error
	DropTable(ctx context.Context, dbName, schema, table string) error
	TruncateTable(ctx context.Context, dbName, schema, table string, cascade bool) error
	AddColumn(ctx context.Context, dbName string, req AddColumnReq) error
	AlterColumn(ctx context.Context, dbName string, req AlterColumnReq) error
	DropColumn(ctx context.Context, dbName, schema, table, column string) error
	CreateIndex(ctx context.Context, dbName string, req CreateIndexReq) error
	DropIndex(ctx context.Context, dbName, schema, table, indexName string) error
	AddConstraint(ctx context.Context, dbName string, req AddConstraintReq) error
	DropConstraint(ctx context.Context, dbName, schema, table, constraintName string) error

	// Row CRUD, Export/Import & Querying
	QueryRows(ctx context.Context, dbName string, req QueryRowsReq) (*QueryRowsResp, error)
	StreamRows(ctx context.Context, dbName string, req QueryRowsReq, allRows bool, onHeader func([]ColumnInfo) error, onRow func([]any) error) error
	InsertRow(ctx context.Context, dbName, schema, table string, values map[string]any) (map[string]any, error)
	ImportRows(ctx context.Context, dbName, schema, table string, req ImportRowsReq) (*ImportResult, error)
	UpdateRow(ctx context.Context, dbName, schema, table string, locator, updates map[string]any) (map[string]any, error)
	DeleteRows(ctx context.Context, dbName, schema, table string, locators []map[string]any) (int64, error)

	// SQL Runner & Explain
	ExecuteSQL(ctx context.Context, dbName, sqlText string) (*SQLResult, error)
	ExplainSQL(ctx context.Context, dbName, sqlText string, analyze bool) (*ExplainResult, error)

	// Activity, Locks, Maintenance & Objects Browser
	GetActivityOverview(ctx context.Context, dbName string) (*ActivityOverview, error)
	CancelBackend(ctx context.Context, pid int, terminate bool) error
	MaintainTable(ctx context.Context, dbName, schema, table, action string) (*SQLResult, error)
	ListDatabaseObjects(ctx context.Context, dbName, schema string) (*DatabaseObjects, error)
	ManageExtension(ctx context.Context, dbName, name, schema, action string) error
	ListAutocompleteCatalog(ctx context.Context, dbName string) ([]AutocompleteTable, error)
}

// HumanBytes formats byte counts cleanly for dashboards.
func HumanBytes(b int64) string {
	if b < 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.2f %s", float64(b)/float64(div), units[exp])
}

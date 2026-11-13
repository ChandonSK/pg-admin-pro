package engine

// EngineKind identifies a supported database engine.
type EngineKind string

const (
	EnginePostgres EngineKind = "postgres"
	EngineMySQL    EngineKind = "mysql"
	EngineSQLite   EngineKind = "sqlite"
)

// ConnectionConfig describes how to connect to a database server or file.
type ConnectionConfig struct {
	Engine   EngineKind `json:"engine"`
	Host     string     `json:"host"`
	Port     int        `json:"port"`
	Database string     `json:"database"`
	Username string     `json:"username"`
	Password string     `json:"password"`
	SSLMode  string     `json:"sslMode"`
}

// PublicConnectionInfo exposes non-secret connection info to the UI.
type PublicConnectionInfo struct {
	Engine   EngineKind `json:"engine"`
	Host     string     `json:"host"`
	Port     int        `json:"port"`
	Database string     `json:"database"`
	Username string     `json:"username"`
	SSLMode  string     `json:"sslMode"`
}

// Capabilities describes what features the connected engine supports.
type Capabilities struct {
	MultiDatabase   bool `json:"multiDatabase"`
	UserManagement  bool `json:"userManagement"`
	Schemas         bool `json:"schemas"`
	BinaryBackup    bool `json:"binaryBackup"`
	RoleAssignment  bool `json:"roleAssignment"`
	ActivityMonitor bool `json:"activityMonitor"`
	ObjectsBrowser  bool `json:"objectsBrowser"`
	ExplainPlan     bool `json:"explainPlan"`
}

// ServerOverview summarizes cluster/server metrics for the dashboard.
type ServerOverview struct {
	Engine          EngineKind   `json:"engine"`
	Version         string       `json:"version"`
	CurrentUser     string       `json:"currentUser"`
	CurrentDatabase string       `json:"currentDatabase"`
	IsAdmin         bool         `json:"isAdmin"`
	TotalDatabases  int          `json:"totalDatabases"`
	TotalUsers      int          `json:"totalUsers"`
	ActiveConns     int          `json:"activeConns"`
	TotalSizeBytes  int64        `json:"totalSizeBytes"`
	TotalSizePretty string       `json:"totalSizePretty"`
	UptimeSeconds   int64        `json:"uptimeSeconds"`
	Capabilities    Capabilities `json:"capabilities"`
}

// DatabaseInfo represents one database on the server.
type DatabaseInfo struct {
	Name          string   `json:"name"`
	Owner         string   `json:"owner"`
	Encoding      string   `json:"encoding"`
	Collation     string   `json:"collation"`
	SizeBytes     int64    `json:"sizeBytes"`
	SizePretty    string   `json:"sizePretty"`
	ActiveConns   int      `json:"activeConns"`
	AllowConnect  bool     `json:"allowConnect"`
	AssignedUsers []string `json:"assignedUsers"`
}

// CreateDatabaseReq defines inputs for creating a database.
type CreateDatabaseReq struct {
	Name     string `json:"name"`
	Owner    string `json:"owner"`
	Encoding string `json:"encoding"`
}

// UserInfo represents a database user / role.
type UserInfo struct {
	Name           string   `json:"name"`
	IsSuperuser    bool     `json:"isSuperuser"`
	CanLogin       bool     `json:"canLogin"`
	CanCreateDB    bool     `json:"canCreateDb"`
	CanCreateRole  bool     `json:"canCreateRole"`
	ConnLimit      int      `json:"connLimit"`
	ValidUntil     *string  `json:"validUntil,omitempty"`
	OwnedDatabases []string `json:"ownedDatabases"`
}

// CreateUserReq defines inputs for creating a database user/role.
type CreateUserReq struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	IsSuperuser   bool   `json:"isSuperuser"`
	CanLogin      bool   `json:"canLogin"`
	CanCreateDB   bool   `json:"canCreateDb"`
	CanCreateRole bool   `json:"canCreateRole"`
	ConnLimit     int    `json:"connLimit"`
}

// UpdateUserReq defines partial updates to an existing user/role.
type UpdateUserReq struct {
	Password      *string `json:"password,omitempty"`
	IsSuperuser   *bool   `json:"isSuperuser,omitempty"`
	CanLogin      *bool   `json:"canLogin,omitempty"`
	CanCreateDB   *bool   `json:"canCreateDb,omitempty"`
	CanCreateRole *bool   `json:"canCreateRole,omitempty"`
	ConnLimit     *int    `json:"connLimit,omitempty"`
}

// DatabaseGrant shows a role's privileges on a specific database.
type DatabaseGrant struct {
	Username     string `json:"username"`
	IsOwner      bool   `json:"isOwner"`
	CanConnect   bool   `json:"canConnect"`
	CanCreate    bool   `json:"canCreate"`
	CanTemp      bool   `json:"canTemp"`
	SchemaAccess string `json:"schemaAccess"`
}

// AssignUserReq grants or changes a user's role/privileges on a database.
type AssignUserReq struct {
	Username          string `json:"username"`
	Preset            string `json:"preset"` // "owner", "admin", "read_write", "read_only", "connect_only"
	Schema            string `json:"schema"` // default "public" for postgres
	ApplyDefaultPrivs bool   `json:"applyDefaultPrivs"`
}

// SchemaInfo describes a schema within a database.
type SchemaInfo struct {
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	TableCount int    `json:"tableCount"`
}

// TableInfo describes a table or view inside a schema.
type TableInfo struct {
	Schema      string `json:"schema"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	RowEstimate int64  `json:"rowEstimate"`
	SizeBytes   int64  `json:"sizeBytes"`
	TotalSize   string `json:"totalSize"`
	Comment     string `json:"comment,omitempty"`
}

// ForeignKeyRef describes a column's foreign key target.
type ForeignKeyRef struct {
	ConstraintName string `json:"constraintName"`
	RefSchema      string `json:"refSchema"`
	RefTable       string `json:"refTable"`
	RefColumn      string `json:"refColumn"`
}

// ColumnInfo describes a column's schema and type metadata.
type ColumnInfo struct {
	Name         string         `json:"name"`
	DataType     string         `json:"dataType"`
	FormatType   string         `json:"formatType"`
	IsNullable   bool           `json:"isNullable"`
	DefaultValue *string        `json:"defaultValue,omitempty"`
	HasDefault   bool           `json:"hasDefault"`
	IsPrimaryKey bool           `json:"isPrimaryKey"`
	IsIdentity   bool           `json:"isIdentity"`
	IsGenerated  bool           `json:"isGenerated"`
	ForeignKey   *ForeignKeyRef `json:"foreignKey,omitempty"`
	EnumValues   []string       `json:"enumValues,omitempty"`
	OrdinalPos   int            `json:"ordinalPos"`
}

// IndexInfo describes an index on a table.
type IndexInfo struct {
	Name       string `json:"name"`
	IsUnique   bool   `json:"isUnique"`
	IsPrimary  bool   `json:"isPrimary"`
	Columns    string `json:"columns"`
	Definition string `json:"definition"`
}

// ConstraintInfo describes a table constraint (PK, FK, UNIQUE, CHECK).
type ConstraintInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Columns    string `json:"columns"`
	Definition string `json:"definition"`
}

// TableStructure holds full structural metadata for a table.
type TableStructure struct {
	Schema            string           `json:"schema"`
	Table             string           `json:"table"`
	Type              string           `json:"type"`
	Columns           []ColumnInfo     `json:"columns"`
	Indexes           []IndexInfo      `json:"indexes"`
	Constraints       []ConstraintInfo `json:"constraints"`
	PrimaryKeys       []string         `json:"primaryKeys"`
	UsesRowIDFallback bool             `json:"usesRowIdFallback"`
}

// FilterCond defines one filter predicate on a column.
type FilterCond struct {
	Column   string `json:"column"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// QueryRowsReq controls paginated table browsing.
type QueryRowsReq struct {
	Schema   string       `json:"schema"`
	Table    string       `json:"table"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
	SortBy   string       `json:"sortBy"`
	SortDir  string       `json:"sortDir"`
	Search   string       `json:"search"`
	Filters  []FilterCond `json:"filters"`
}

// QueryRowsResp returns paginated table rows and key locators for safe updates/deletes.
type QueryRowsResp struct {
	Columns           []ColumnInfo     `json:"columns"`
	PrimaryKeys       []string         `json:"primaryKeys"`
	UsesRowIDFallback bool             `json:"usesRowIdFallback"`
	Rows              []map[string]any `json:"rows"`
	RowLocators       []map[string]any `json:"rowLocators"`
	TotalCount        int64            `json:"totalCount"`
	Page              int              `json:"page"`
	PageSize          int              `json:"pageSize"`
	DurationMs        int64            `json:"durationMs"`
}

// CreateColumnDef defines a column when creating a new table.
type CreateColumnDef struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primaryKey"`
	Default    string `json:"default"`
}

// CreateTableReq defines inputs for creating a new table.
type CreateTableReq struct {
	Schema  string            `json:"schema"`
	Table   string            `json:"table"`
	Columns []CreateColumnDef `json:"columns"`
}

// SQLResult represents the result of an arbitrary SQL execution.
type SQLResult struct {
	Columns      []string `json:"columns"`
	Rows         [][]any  `json:"rows"`
	RowsAffected int64    `json:"rowsAffected"`
	DurationMs   int64    `json:"durationMs"`
	CommandTag   string   `json:"commandTag"`
	IsSelect     bool     `json:"isSelect"`
}

// AddColumnReq defines inputs for ALTER TABLE ... ADD COLUMN.
type AddColumnReq struct {
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default"`
}

// AlterColumnReq defines partial structural changes to an existing column.
type AlterColumnReq struct {
	Schema      string  `json:"schema"`
	Table       string  `json:"table"`
	Column      string  `json:"column"`
	NewName     string  `json:"newName,omitempty"`
	NewType     string  `json:"newType,omitempty"`
	SetNullable *bool   `json:"setNullable,omitempty"`
	SetDefault  *string `json:"setDefault,omitempty"`
	DropDefault bool    `json:"dropDefault,omitempty"`
}

// CreateIndexReq defines inputs for creating a new index on a table.
type CreateIndexReq struct {
	Schema  string   `json:"schema"`
	Table   string   `json:"table"`
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Method  string   `json:"method"`
}

// AddConstraintReq defines inputs for adding FK, UNIQUE, or CHECK constraints.
type AddConstraintReq struct {
	Schema     string   `json:"schema"`
	Table      string   `json:"table"`
	Name       string   `json:"name"`
	Type       string   `json:"type"` // FOREIGN KEY, UNIQUE, CHECK
	Columns    []string `json:"columns"`
	RefSchema  string   `json:"refSchema"`
	RefTable   string   `json:"refTable"`
	RefColumns []string `json:"refColumns"`
	OnDelete   string   `json:"onDelete"`
	OnUpdate   string   `json:"onUpdate"`
	CheckExpr  string   `json:"checkExpr"`
}

// ImportRowsReq holds parsed rows uploaded from CSV/JSON for bulk insertion.
type ImportRowsReq struct {
	Rows            []map[string]any `json:"rows"`
	ContinueOnError bool             `json:"continueOnError"`
}

// ImportRowError describes one failed row during bulk import.
type ImportRowError struct {
	RowIndex int    `json:"rowIndex"`
	Error    string `json:"error"`
}

// ImportResult reports bulk import outcome.
type ImportResult struct {
	Inserted   int64            `json:"inserted"`
	Failed     int64            `json:"failed"`
	DurationMs int64            `json:"durationMs"`
	Errors     []ImportRowError `json:"errors,omitempty"`
}

// ServerMetrics summarizes database/cluster cache & transaction health.
type ServerMetrics struct {
	CacheHitRatio   float64 `json:"cacheHitRatio"`
	ActiveCount     int     `json:"activeCount"`
	IdleCount       int     `json:"idleCount"`
	WaitingCount    int     `json:"waitingCount"`
	MaxConnections  int     `json:"maxConnections"`
	Commits         int64   `json:"commits"`
	Rollbacks       int64   `json:"rollbacks"`
	Deadlocks       int64   `json:"deadlocks"`
	TempBytesPretty string  `json:"tempBytesPretty"`
}

// ActivitySession represents an active backend connection/query.
type ActivitySession struct {
	PID          int    `json:"pid"`
	Database     string `json:"database"`
	Username     string `json:"username"`
	AppName      string `json:"appName"`
	ClientAddr   string `json:"clientAddr"`
	State        string `json:"state"`
	WaitEvent    string `json:"waitEvent"`
	DurationMs   int64  `json:"durationMs"`
	QueryStart   string `json:"queryStart"`
	Query        string `json:"query"`
	BlockingPIDs []int  `json:"blockingPids,omitempty"`
}

// LockInfo describes a held or waiting lock on a database object.
type LockInfo struct {
	PID      int    `json:"pid"`
	Database string `json:"database"`
	Relation string `json:"relation"`
	LockType string `json:"lockType"`
	Mode     string `json:"mode"`
	Granted  bool   `json:"granted"`
	State    string `json:"state"`
	Query    string `json:"query"`
}

// TablePerfInfo holds storage size, bloat (dead tuples), and scan counters.
type TablePerfInfo struct {
	Schema          string `json:"schema"`
	Table           string `json:"table"`
	TotalSizeBytes  int64  `json:"totalSizeBytes"`
	TotalSizePretty string `json:"totalSizePretty"`
	TableSizePretty string `json:"tableSizePretty"`
	IndexSizePretty string `json:"indexSizePretty"`
	LiveTuples      int64  `json:"liveTuples"`
	DeadTuples      int64  `json:"deadTuples"`
	SeqScans        int64  `json:"seqScans"`
	IdxScans        int64  `json:"idxScans"`
	LastVacuum      string `json:"lastVacuum,omitempty"`
	LastAnalyze     string `json:"lastAnalyze,omitempty"`
}

// UnusedIndexInfo identifies indexes with zero scans consuming disk/write overhead.
type UnusedIndexInfo struct {
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	IndexName  string `json:"indexName"`
	Scans      int64  `json:"scans"`
	SizeBytes  int64  `json:"sizeBytes"`
	SizePretty string `json:"sizePretty"`
}

// ActivityOverview bundles live sessions, locks, cache metrics, and table stats.
type ActivityOverview struct {
	Metrics       ServerMetrics     `json:"metrics"`
	Sessions      []ActivitySession `json:"sessions"`
	Locks         []LockInfo        `json:"locks"`
	TableStats    []TablePerfInfo   `json:"tableStats"`
	UnusedIndexes []UnusedIndexInfo `json:"unusedIndexes"`
}

// ExplainResult holds query execution plan output.
type ExplainResult struct {
	PlanJSON   any        `json:"planJson,omitempty"`
	PlanLines  []string   `json:"planLines,omitempty"`
	DurationMs int64      `json:"durationMs"`
	Analyzed   bool       `json:"analyzed"`
	Engine     EngineKind `json:"engine"`
}

// DBFunctionInfo represents a stored function or procedure.
type DBFunctionInfo struct {
	Schema     string `json:"schema"`
	Name       string `json:"name"`
	Args       string `json:"args"`
	ReturnType string `json:"returnType"`
	Language   string `json:"language"`
	Kind       string `json:"kind"`
	Definition string `json:"definition"`
}

// DBTriggerInfo represents a table trigger.
type DBTriggerInfo struct {
	Schema     string `json:"schema"`
	Name       string `json:"name"`
	Table      string `json:"table"`
	Event      string `json:"event"`
	Enabled    bool   `json:"enabled"`
	Definition string `json:"definition"`
}

// DBSequenceInfo represents a database sequence.
type DBSequenceInfo struct {
	Schema      string `json:"schema"`
	Name        string `json:"name"`
	DataType    string `json:"dataType"`
	LastValue   *int64 `json:"lastValue"`
	IncrementBy int64  `json:"incrementBy"`
	MinValue    int64  `json:"minValue"`
	MaxValue    int64  `json:"maxValue"`
}

// DBTypeInfo represents a custom enum, composite, or domain type.
type DBTypeInfo struct {
	Schema     string   `json:"schema"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Values     []string `json:"values,omitempty"`
	Definition string   `json:"definition,omitempty"`
}

// DBExtensionInfo describes an available or installed PostgreSQL extension.
type DBExtensionInfo struct {
	Name             string `json:"name"`
	InstalledVersion string `json:"installedVersion"`
	DefaultVersion   string `json:"defaultVersion"`
	Schema           string `json:"schema"`
	Comment          string `json:"comment"`
	Installed        bool   `json:"installed"`
}

// DatabaseObjects bundles non-table objects for a schema/database.
type DatabaseObjects struct {
	Functions  []DBFunctionInfo  `json:"functions"`
	Triggers   []DBTriggerInfo   `json:"triggers"`
	Sequences  []DBSequenceInfo  `json:"sequences"`
	Types      []DBTypeInfo      `json:"types"`
	Extensions []DBExtensionInfo `json:"extensions"`
}

// AutocompleteTable provides table and column names for SQL IDE completion.
type AutocompleteTable struct {
	Schema  string   `json:"schema"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Columns []string `json:"columns"`
}

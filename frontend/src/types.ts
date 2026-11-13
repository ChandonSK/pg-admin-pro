export type EngineKind = 'postgres' | 'mysql' | 'sqlite';

export interface PublicConnectionInfo {
  engine: EngineKind;
  host: string;
  port: number;
  database: string;
  username: string;
  sslMode: string;
}

export interface ConnectionDefaults {
  engine: EngineKind;
  host: string;
  port: number;
  database: string;
  username: string;
  sslMode: string;
}

export interface Capabilities {
  multiDatabase: boolean;
  userManagement: boolean;
  schemas: boolean;
  binaryBackup: boolean;
  roleAssignment: boolean;
  activityMonitor: boolean;
  objectsBrowser: boolean;
  explainPlan: boolean;
}

export interface ServerOverview {
  engine: EngineKind;
  version: string;
  currentUser: string;
  currentDatabase: string;
  isAdmin: boolean;
  totalDatabases: number;
  totalUsers: number;
  activeConns: number;
  totalSizeBytes: number;
  totalSizePretty: string;
  uptimeSeconds: number;
  capabilities: Capabilities;
}

export interface DatabaseInfo {
  name: string;
  owner: string;
  encoding: string;
  collation: string;
  sizeBytes: number;
  sizePretty: string;
  activeConns: number;
  allowConnect: boolean;
  assignedUsers: string[];
}

export interface UserInfo {
  name: string;
  isSuperuser: boolean;
  canLogin: boolean;
  canCreateDb: boolean;
  canCreateRole: boolean;
  connLimit: number;
  validUntil?: string | null;
  ownedDatabases: string[];
}

export interface DatabaseGrant {
  username: string;
  isOwner: boolean;
  canConnect: boolean;
  canCreate: boolean;
  canTemp: boolean;
  schemaAccess: string;
}

export interface SchemaInfo {
  name: string;
  owner: string;
  tableCount: number;
}

export interface TableInfo {
  schema: string;
  name: string;
  type: string;
  rowEstimate: number;
  sizeBytes: number;
  totalSize: string;
  comment?: string;
}

export interface ForeignKeyRef {
  constraintName: string;
  refSchema: string;
  refTable: string;
  refColumn: string;
}

export interface ColumnInfo {
  name: string;
  dataType: string;
  formatType: string;
  isNullable: boolean;
  defaultValue?: string | null;
  hasDefault: boolean;
  isPrimaryKey: boolean;
  isIdentity: boolean;
  isGenerated: boolean;
  foreignKey?: ForeignKeyRef | null;
  enumValues?: string[];
  ordinalPos: number;
}

export interface IndexInfo {
  name: string;
  isUnique: boolean;
  isPrimary: boolean;
  columns: string;
  definition: string;
}

export interface ConstraintInfo {
  name: string;
  type: string;
  columns: string;
  definition: string;
}

export interface TableStructure {
  schema: string;
  table: string;
  type: string;
  columns: ColumnInfo[];
  indexes: IndexInfo[];
  constraints: ConstraintInfo[];
  primaryKeys: string[];
  usesRowIdFallback: boolean;
}

export interface FilterCond {
  column: string;
  operator: string;
  value: string;
}

export interface QueryRowsResp {
  columns: ColumnInfo[];
  primaryKeys: string[];
  usesRowIdFallback: boolean;
  rows: Record<string, unknown>[];
  rowLocators: Record<string, unknown>[];
  totalCount: number;
  page: number;
  pageSize: number;
  durationMs: number;
}

export interface SQLResult {
  columns: string[];
  rows: unknown[][];
  rowsAffected: number;
  durationMs: number;
  commandTag: string;
  isSelect: boolean;
}

export interface ToolCapabilities {
  pgDumpAvailable: boolean;
  pgDumpVersion?: string;
  pgRestoreAvailable: boolean;
  pgRestoreVersion?: string;
  psqlAvailable: boolean;
  mysqlDumpAvailable: boolean;
  mysqlClientAvailable: boolean;
  s3Configured: boolean;
  backupDir: string;
}

export interface BackupFile {
  filename: string;
  database: string;
  engine: string;
  format: string;
  sizeBytes: number;
  sizePretty: string;
  createdAt: string;
}

export interface BackupResult {
  file: BackupFile;
  durationMs: number;
  command: string;
}

export interface RestoreResult {
  filename: string;
  database: string;
  durationMs: number;
  output: string;
}

export type ProfileEnv = 'local' | 'staging' | 'production' | 'other';

export interface ServerProfile {
  id: string;
  name: string;
  env: ProfileEnv;
  engine: EngineKind;
  host: string;
  port: number;
  database: string;
  username: string;
  sslMode: string;
  hasPassword: boolean;
  createdAt: string;
  lastUsedAt?: string;
}

export interface ProfileUpsertReq {
  id?: string;
  name: string;
  env: ProfileEnv;
  engine: EngineKind;
  host: string;
  port: number;
  database: string;
  username: string;
  password?: string;
  savePassword: boolean;
  keepPassword?: boolean;
  sslMode: string;
}

export interface SQLSnippet {
  id: string;
  title: string;
  tag: string;
  sql: string;
  engine: string;
  updatedAt: string;
}

export interface ImportResult {
  inserted: number;
  failed: number;
  durationMs: number;
  errors?: { rowIndex: number; error: string }[];
}

export interface ServerMetrics {
  cacheHitRatio: number;
  activeCount: number;
  idleCount: number;
  waitingCount: number;
  maxConnections: number;
  commits: number;
  rollbacks: number;
  deadlocks: number;
  tempBytesPretty: string;
}

export interface ActivitySession {
  pid: number;
  database: string;
  username: string;
  appName: string;
  clientAddr: string;
  state: string;
  waitEvent: string;
  durationMs: number;
  queryStart: string;
  query: string;
  blockingPids?: number[];
}

export interface LockInfo {
  pid: number;
  database: string;
  relation: string;
  lockType: string;
  mode: string;
  granted: boolean;
  state: string;
  query: string;
}

export interface TablePerfInfo {
  schema: string;
  table: string;
  totalSizeBytes: number;
  totalSizePretty: string;
  tableSizePretty: string;
  indexSizePretty: string;
  liveTuples: number;
  deadTuples: number;
  seqScans: number;
  idxScans: number;
  lastVacuum?: string;
  lastAnalyze?: string;
}

export interface UnusedIndexInfo {
  schema: string;
  table: string;
  indexName: string;
  scans: number;
  sizeBytes: number;
  sizePretty: string;
}

export interface ActivityOverview {
  metrics: ServerMetrics;
  sessions: ActivitySession[];
  locks: LockInfo[];
  tableStats: TablePerfInfo[];
  unusedIndexes: UnusedIndexInfo[];
}

export interface ExplainResult {
  planJson?: unknown;
  planLines?: string[];
  durationMs: number;
  analyzed: boolean;
  engine: EngineKind;
}

export interface DBFunctionInfo {
  schema: string;
  name: string;
  args: string;
  returnType: string;
  language: string;
  kind: string;
  definition: string;
}

export interface DBTriggerInfo {
  schema: string;
  name: string;
  table: string;
  event: string;
  enabled: boolean;
  definition: string;
}

export interface DBSequenceInfo {
  schema: string;
  name: string;
  dataType: string;
  lastValue: number | null;
  incrementBy: number;
  minValue: number;
  maxValue: number;
}

export interface DBTypeInfo {
  schema: string;
  name: string;
  kind: string;
  values?: string[];
  definition?: string;
}

export interface DBExtensionInfo {
  name: string;
  installedVersion: string;
  defaultVersion: string;
  schema: string;
  comment: string;
  installed: boolean;
}

export interface DatabaseObjects {
  functions: DBFunctionInfo[];
  triggers: DBTriggerInfo[];
  sequences: DBSequenceInfo[];
  types: DBTypeInfo[];
  extensions: DBExtensionInfo[];
}

export interface AutocompleteTable {
  schema: string;
  name: string;
  type: string;
  columns: string[];
}

export interface BackupSettingsView {
  autoBackupEnabled: boolean;
  autoBackupTime: string;
  autoBackupDatabase: string;
  compression: number;
  retentionDays: number;
  maxBackups: number;
  autoHost: string;
  autoPort: number;
  autoDatabase: string;
  autoUsername: string;
  autoSslMode: string;
  hasAutoPassword: boolean;
  s3Enabled: boolean;
  s3Endpoint: string;
  s3Region: string;
  s3Bucket: string;
  s3Prefix: string;
  s3AccessKeyId: string;
  hasS3SecretKey: boolean;
  s3ForcePathStyle: boolean;
  s3AutoUpload: boolean;
  lastRunAt?: string;
  lastRunStatus?: string;
  lastRunMessage?: string;
}

export interface BackupSettingsUpdateReq {
  autoBackupEnabled: boolean;
  autoBackupTime: string;
  autoBackupDatabase: string;
  compression: number;
  retentionDays: number;
  maxBackups: number;
  autoHost: string;
  autoPort: number;
  autoDatabase: string;
  autoUsername: string;
  autoPassword?: string;
  clearAutoPassword?: boolean;
  autoSslMode: string;
  s3Enabled: boolean;
  s3Endpoint: string;
  s3Region: string;
  s3Bucket: string;
  s3Prefix: string;
  s3AccessKeyId: string;
  s3SecretKey?: string;
  clearS3SecretKey?: boolean;
  s3ForcePathStyle: boolean;
  s3AutoUpload: boolean;
}

export interface S3Object {
  key: string;
  filename: string;
  sizeBytes: number;
  sizePretty: string;
  lastModified: string;
  etag: string;
}

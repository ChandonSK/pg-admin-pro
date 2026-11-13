import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  ArrowLeft,
  Blocks,
  Code2,
  Columns3,
  Database,
  Eye,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Shield,
  Table2,
  Trash2,
} from 'lucide-react';
import { api } from '../api/client';
import type { FilterCond, SchemaInfo, ServerOverview, TableInfo, UserInfo } from '../types';
import { DatabaseGrantsPanel } from './DatabaseGrantsPanel';
import { DatabaseObjectsPanel } from './DatabaseObjectsPanel';
import { Modal } from './Modal';
import { SqlEditorTab } from './SqlEditorTab';
import { TableDataTab } from './TableDataTab';
import { TableStructureTab } from './TableStructureTab';

interface DatabaseWorkspaceProps {
  dbName: string;
  overview: ServerOverview;
  users: UserInfo[];
  onBack: () => void;
  onNotify: (kind: 'success' | 'error' | 'info', msg: string) => void;
  onRefreshCluster: () => void;
}

type WorkspaceMode = 'data' | 'structure' | 'objects' | 'sql' | 'permissions';

export function DatabaseWorkspace({
  dbName,
  overview,
  users,
  onBack,
  onNotify,
  onRefreshCluster,
}: DatabaseWorkspaceProps) {
  const [schemas, setSchemas] = useState<SchemaInfo[]>([]);
  const [selectedSchema, setSelectedSchema] = useState(
    overview.engine === 'postgres' ? 'public' : overview.engine === 'sqlite' ? 'main' : dbName
  );
  const [includeSystem, setIncludeSystem] = useState(false);
  const [tables, setTables] = useState<TableInfo[]>([]);
  const [tableSearch, setTableSearch] = useState('');
  const [selectedTable, setSelectedTable] = useState<TableInfo | null>(null);
  const [jumpFilter, setJumpFilter] = useState<FilterCond | null>(null);
  const [mode, setMode] = useState<WorkspaceMode>('data');
  const [loadingTables, setLoadingTables] = useState(true);

  // Create Table modal state
  const [createTableOpen, setCreateTableOpen] = useState(false);
  const [newTableName, setNewTableName] = useState('');
  const [newCols, setNewCols] = useState([
    { name: 'id', type: overview.engine === 'postgres' ? 'BIGSERIAL' : 'INTEGER', nullable: false, primaryKey: true, default: '' },
    { name: 'name', type: 'TEXT', nullable: false, primaryKey: false, default: '' },
    { name: 'created_at', type: overview.engine === 'postgres' ? 'TIMESTAMPTZ' : 'DATETIME', nullable: false, primaryKey: false, default: overview.engine === 'postgres' ? 'now()' : '' },
  ]);
  const [creatingTable, setCreatingTable] = useState(false);

  const loadSchemasAndTables = useCallback(async (
    schemaOverride?: string,
    preferredTable?: { schema: string; name: string }
  ) => {
    setLoadingTables(true);
    try {
      const sRes = await api.tables.schemas(dbName, includeSystem);
      setSchemas(sRes.schemas);
      const activeSchema =
        schemaOverride ||
        (sRes.schemas.some((s) => s.name === selectedSchema)
          ? selectedSchema
          : sRes.schemas[0]?.name || 'public');
      if (activeSchema !== selectedSchema) {
        setSelectedSchema(activeSchema);
      }
      const tRes = await api.tables.list(dbName, activeSchema);
      setTables(tRes.tables);
      setSelectedTable((prev) => {
        const targetSchema = preferredTable?.schema ?? activeSchema;
        const targetName = preferredTable?.name ?? prev?.name;
        if (targetName && tRes.tables.some((t) => t.name === targetName && t.schema === targetSchema)) {
          return tRes.tables.find((t) => t.name === targetName && t.schema === targetSchema) || null;
        }
        return tRes.tables[0] || null;
      });
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to load tables');
    } finally {
      setLoadingTables(false);
    }
  }, [dbName, includeSystem, selectedSchema, onNotify]);

  useEffect(() => {
    setSelectedTable(null);
    setJumpFilter(null);
    setMode('data');
    loadSchemasAndTables();
  }, [dbName, includeSystem]);

  const handleSchemaChange = async (schema: string) => {
    setSelectedSchema(schema);
    setSelectedTable(null);
    setJumpFilter(null);
    await loadSchemasAndTables(schema);
  };

  const handleJumpForeignKey = useCallback(
    async (refSchema: string, refTable: string, refColumn: string, value: string) => {
      const targetSchema = refSchema || selectedSchema;
      const nextFilter: FilterCond = { column: refColumn, operator: '=', value };
      if (targetSchema === selectedSchema) {
        const match = tables.find((t) => t.schema === targetSchema && t.name === refTable);
        if (match) {
          setSelectedTable(match);
          setJumpFilter(nextFilter);
          setMode('data');
          return;
        }
      }
      setSelectedSchema(targetSchema);
      await loadSchemasAndTables(targetSchema, { schema: targetSchema, name: refTable });
      setJumpFilter(nextFilter);
      setMode('data');
    },
    [selectedSchema, tables, loadSchemasAndTables]
  );

  const handleCreateTable = async (e: FormEvent) => {
    e.preventDefault();
    if (!newTableName.trim()) return;
    setCreatingTable(true);
    try {
      await api.tables.create(dbName, {
        schema: selectedSchema,
        table: newTableName.trim(),
        columns: newCols,
      });
      onNotify('success', `Table "${newTableName.trim()}" created`);
      setCreateTableOpen(false);
      setNewTableName('');
      await loadSchemasAndTables(selectedSchema);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Create table failed');
    } finally {
      setCreatingTable(false);
    }
  };

  const filteredTables = tables.filter((t) =>
    t.name.toLowerCase().includes(tableSearch.toLowerCase())
  );

  return (
    <div className="flex flex-col lg:flex-row gap-4 min-h-[calc(100vh-8.5rem)]">
      {/* Left Sidebar: Schema & Tables */}
      <aside className="w-full lg:w-72 shrink-0 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 flex flex-col overflow-hidden">
        <div className="p-3.5 border-b border-slate-200 dark:border-slate-800 space-y-3">
          <div className="flex items-center justify-between gap-2">
            <button
              type="button"
              onClick={onBack}
              className="inline-flex items-center gap-1 text-xs text-slate-500 hover:text-indigo-500 font-medium"
            >
              <ArrowLeft className="w-3.5 h-3.5" /> All Databases
            </button>
            <button
              type="button"
              onClick={() => loadSchemasAndTables()}
              className="p-1 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 text-slate-400"
              aria-label="Refresh tables"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${loadingTables ? 'animate-spin' : ''}`} />
            </button>
          </div>

          <div className="flex items-center gap-2">
            <Database className="w-4 h-4 text-indigo-500 shrink-0" />
            <span className="font-mono font-bold text-sm truncate text-slate-900 dark:text-white">
              {dbName}
            </span>
          </div>

          {overview.capabilities.schemas && (
            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-medium text-slate-500">Schema</label>
                <label className="inline-flex items-center gap-1 text-[10px] text-slate-400 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={includeSystem}
                    onChange={(e) => setIncludeSystem(e.target.checked)}
                  />
                  System
                </label>
              </div>
              <select
                value={selectedSchema}
                onChange={(e) => handleSchemaChange(e.target.value)}
                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
              >
                {schemas.map((s) => (
                  <option key={s.name} value={s.name}>
                    {s.name} ({s.tableCount})
                  </option>
                ))}
              </select>
            </div>
          )}

          <div className="flex items-center gap-1.5">
            <div className="relative flex-1">
              <Search className="w-3.5 h-3.5 text-slate-400 absolute left-2.5 top-2" />
              <input
                type="text"
                value={tableSearch}
                onChange={(e) => setTableSearch(e.target.value)}
                placeholder="Filter tables..."
                className="w-full pl-8 pr-2 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 text-xs focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
            <button
              type="button"
              onClick={() => setCreateTableOpen(true)}
              title="Create new table"
              className="p-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white"
            >
              <Plus className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Table list */}
        <div className="flex-1 overflow-y-auto p-2 space-y-0.5 max-h-[420px] lg:max-h-none">
          {filteredTables.map((t) => {
            const active =
              selectedTable?.name === t.name &&
              selectedTable?.schema === t.schema &&
              (mode === 'data' || mode === 'structure');
            const isView = t.type.includes('VIEW');
            return (
              <button
                key={`${t.schema}.${t.name}`}
                type="button"
                onClick={() => {
                  setJumpFilter(null);
                  setSelectedTable(t);
                  if (mode === 'sql' || mode === 'permissions' || mode === 'objects') setMode('data');
                }}
                className={`w-full flex items-center justify-between gap-2 px-2.5 py-2 rounded-xl text-left text-xs transition-colors ${
                  active
                    ? 'bg-indigo-600 text-white'
                    : 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/70'
                }`}
              >
                <div className="flex items-center gap-2 min-w-0">
                  {isView ? (
                    <Eye className="w-3.5 h-3.5 shrink-0 opacity-75" />
                  ) : (
                    <Table2 className="w-3.5 h-3.5 shrink-0 opacity-75" />
                  )}
                  <span className="font-mono truncate">{t.name}</span>
                </div>
                <span
                  className={`text-[10px] font-mono shrink-0 px-1.5 py-0.5 rounded ${
                    active ? 'bg-white/20 text-white' : 'bg-slate-100 dark:bg-slate-800 text-slate-400'
                  }`}
                >
                  {t.rowEstimate.toLocaleString()}
                </span>
              </button>
            );
          })}
          {!loadingTables && filteredTables.length === 0 && (
            <div className="py-8 text-center text-xs text-slate-500">
              No tables in schema "{selectedSchema}".
            </div>
          )}
        </div>

        {/* Sidebar footer quick tools */}
        <div className="p-2.5 border-t border-slate-200 dark:border-slate-800 flex flex-wrap gap-1.5">
          <button
            type="button"
            onClick={() => setMode('sql')}
            className={`flex-1 min-w-[78px] flex items-center justify-center gap-1.5 py-1.5 px-2 rounded-lg text-xs font-medium ${
              mode === 'sql'
                ? 'bg-indigo-600 text-white'
                : 'bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700'
            }`}
          >
            <Code2 className="w-3.5 h-3.5" /> SQL
          </button>
          {overview.capabilities.objectsBrowser && (
            <button
              type="button"
              onClick={() => setMode('objects')}
              className={`flex-1 min-w-[78px] flex items-center justify-center gap-1.5 py-1.5 px-2 rounded-lg text-xs font-medium ${
                mode === 'objects'
                  ? 'bg-indigo-600 text-white'
                  : 'bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700'
              }`}
            >
              <Blocks className="w-3.5 h-3.5" /> Objects
            </button>
          )}
          {overview.capabilities.roleAssignment && (
            <button
              type="button"
              onClick={() => setMode('permissions')}
              className={`flex-1 min-w-[78px] flex items-center justify-center gap-1.5 py-1.5 px-2 rounded-lg text-xs font-medium ${
                mode === 'permissions'
                  ? 'bg-indigo-600 text-white'
                  : 'bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700'
              }`}
            >
              <Shield className="w-3.5 h-3.5" /> Users
            </button>
          )}
        </div>
      </aside>

      {/* Main Content Area */}
      <section className="flex-1 min-w-0 flex flex-col space-y-3">
        {/* Top Mode Tabs */}
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 px-4 py-2.5">
          <div className="flex flex-wrap items-center gap-1.5">
            {selectedTable && (
              <>
                <button
                  type="button"
                  onClick={() => setMode('data')}
                  className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium ${
                    mode === 'data'
                      ? 'bg-indigo-600 text-white'
                      : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <Table2 className="w-3.5 h-3.5" /> Data
                </button>
                <button
                  type="button"
                  onClick={() => setMode('structure')}
                  className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium ${
                    mode === 'structure'
                      ? 'bg-indigo-600 text-white'
                      : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                  }`}
                >
                  <Columns3 className="w-3.5 h-3.5" /> Structure
                </button>
              </>
            )}
            {overview.capabilities.objectsBrowser && (
              <button
                type="button"
                onClick={() => setMode('objects')}
                className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium ${
                  mode === 'objects'
                    ? 'bg-indigo-600 text-white'
                    : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <Blocks className="w-3.5 h-3.5" /> Objects
              </button>
            )}
            <button
              type="button"
              onClick={() => setMode('sql')}
              className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium ${
                mode === 'sql'
                  ? 'bg-indigo-600 text-white'
                  : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
              }`}
            >
              <Code2 className="w-3.5 h-3.5" /> SQL Runner
            </button>
            {overview.capabilities.roleAssignment && (
              <button
                type="button"
                onClick={() => setMode('permissions')}
                className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium ${
                  mode === 'permissions'
                    ? 'bg-indigo-600 text-white'
                    : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <Shield className="w-3.5 h-3.5" /> DB Users
              </button>
            )}
          </div>

          {selectedTable && (mode === 'data' || mode === 'structure') && (
            <div className="text-xs font-mono text-slate-500">
              <span className="text-slate-400">{selectedTable.schema}.</span>
              <strong className="text-slate-900 dark:text-white">{selectedTable.name}</strong>
              {selectedTable.totalSize && selectedTable.totalSize !== '-' && (
                <span className="ml-2 text-[11px] text-slate-400">
                  ({selectedTable.totalSize})
                </span>
              )}
            </div>
          )}
        </div>

        {/* Active Panel */}
        <div className="flex-1 min-h-0">
          {mode === 'sql' ? (
            <SqlEditorTab
              dbName={dbName}
              supportsExplain={overview.capabilities.explainPlan}
              onNotify={onNotify}
              initialSql={
                selectedTable
                  ? overview.engine === 'mysql'
                    ? `SELECT * FROM \`${selectedTable.name}\` LIMIT 50;`
                    : `SELECT * FROM "${selectedTable.schema}"."${selectedTable.name}" LIMIT 50;`
                  : undefined
              }
            />
          ) : mode === 'objects' ? (
            <DatabaseObjectsPanel
              dbName={dbName}
              schema={selectedSchema}
              onNotify={onNotify}
            />
          ) : mode === 'permissions' ? (
            <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 p-5">
              <DatabaseGrantsPanel
                dbName={dbName}
                users={users}
                supportsSchemas={overview.capabilities.schemas}
                onNotify={onNotify}
                onChanged={onRefreshCluster}
              />
            </div>
          ) : selectedTable ? (
            mode === 'structure' ? (
              <TableStructureTab
                dbName={dbName}
                schema={selectedTable.schema}
                table={selectedTable.name}
                engineKind={overview.engine}
                onDropped={() => loadSchemasAndTables(selectedSchema)}
                onChanged={() => loadSchemasAndTables(selectedSchema)}
                onNotify={onNotify}
              />
            ) : (
              <TableDataTab
                dbName={dbName}
                schema={selectedTable.schema}
                table={selectedTable.name}
                tableType={selectedTable.type}
                initialFilter={jumpFilter}
                onJumpForeignKey={handleJumpForeignKey}
                onNotify={onNotify}
              />
            )
          ) : (
            <div className="rounded-2xl border border-dashed border-slate-300 dark:border-slate-700 p-12 text-center space-y-3">
              <Table2 className="w-8 h-8 mx-auto text-slate-400" />
              <p className="text-sm font-medium">
                Select a table from the left sidebar or create a new table.
              </p>
              <div className="flex justify-center gap-2">
                <button
                  type="button"
                  onClick={() => setCreateTableOpen(true)}
                  className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold"
                >
                  <Plus className="w-3.5 h-3.5" /> Create Table
                </button>
                <button
                  type="button"
                  onClick={() => setMode('sql')}
                  className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs font-medium"
                >
                  <Code2 className="w-3.5 h-3.5" /> Open SQL Runner
                </button>
              </div>
            </div>
          )}
        </div>
      </section>

      {/* Create Table Modal */}
      <Modal
        open={createTableOpen}
        onClose={() => setCreateTableOpen(false)}
        title={`Create Table in ${dbName}`}
        subtitle={`Target schema: ${selectedSchema}`}
        maxWidthClass="max-w-2xl"
      >
        <form onSubmit={handleCreateTable} className="space-y-4">
          <div>
            <label className="block text-xs font-medium mb-1">Table Name</label>
            <input
              type="text"
              required
              autoFocus
              value={newTableName}
              onChange={(e) => setNewTableName(e.target.value)}
              placeholder="students"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm font-mono"
            />
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold">Columns</span>
              <button
                type="button"
                onClick={() =>
                  setNewCols((prev) => [
                    ...prev,
                    { name: '', type: 'TEXT', nullable: true, primaryKey: false, default: '' },
                  ])
                }
                className="inline-flex items-center gap-1 text-xs text-indigo-500 hover:underline"
              >
                <Plus className="w-3.5 h-3.5" /> Add Column
              </button>
            </div>

            {newCols.map((col, idx) => (
              <div key={idx} className="grid grid-cols-12 gap-2 items-center">
                <input
                  type="text"
                  required
                  placeholder="name"
                  value={col.name}
                  onChange={(e) =>
                    setNewCols((prev) =>
                      prev.map((c, i) => (i === idx ? { ...c, name: e.target.value } : c))
                    )
                  }
                  className="col-span-3 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
                />
                <input
                  type="text"
                  required
                  placeholder="TEXT"
                  value={col.type}
                  onChange={(e) =>
                    setNewCols((prev) =>
                      prev.map((c, i) => (i === idx ? { ...c, type: e.target.value } : c))
                    )
                  }
                  className="col-span-3 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
                />
                <input
                  type="text"
                  placeholder="Default"
                  value={col.default}
                  onChange={(e) =>
                    setNewCols((prev) =>
                      prev.map((c, i) => (i === idx ? { ...c, default: e.target.value } : c))
                    )
                  }
                  className="col-span-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2 py-1.5 text-xs font-mono"
                />
                <label className="col-span-2 flex items-center gap-1 text-[11px]">
                  <input
                    type="checkbox"
                    checked={col.primaryKey}
                    onChange={(e) =>
                      setNewCols((prev) =>
                        prev.map((c, i) => (i === idx ? { ...c, primaryKey: e.target.checked } : c))
                      )
                    }
                  />
                  PK
                </label>
                <label className="col-span-1 flex items-center gap-1 text-[11px]">
                  <input
                    type="checkbox"
                    checked={col.nullable}
                    onChange={(e) =>
                      setNewCols((prev) =>
                        prev.map((c, i) => (i === idx ? { ...c, nullable: e.target.checked } : c))
                      )
                    }
                  />
                  Null
                </label>
                <button
                  type="button"
                  onClick={() =>
                    setNewCols((prev) => prev.filter((_, i) => i !== idx))
                  }
                  disabled={newCols.length <= 1}
                  className="col-span-1 text-red-500 disabled:opacity-30 flex justify-center"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            ))}
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setCreateTableOpen(false)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={creatingTable}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold"
            >
              {creatingTable && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Create Table
            </button>
          </div>
        </form>
      </Modal>
    </div>
  );
}

import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  Eraser,
  Key,
  Link2,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Shield,
  Sparkles,
  Trash2,
} from 'lucide-react';
import { api } from '../api/client';
import type {
  ColumnInfo,
  ConstraintInfo,
  EngineKind,
  IndexInfo,
  TableStructure,
} from '../types';
import { ConfirmModal } from './ConfirmModal';
import { Modal } from './Modal';

interface TableStructureTabProps {
  dbName: string;
  schema: string;
  table: string;
  engineKind: EngineKind;
  onDropped: () => void;
  onChanged?: () => void;
  onNotify: (kind: 'success' | 'error' | 'info', msg: string) => void;
}

const PG_TYPES = [
  'text',
  'varchar(255)',
  'integer',
  'bigint',
  'boolean',
  'numeric(12,2)',
  'double precision',
  'timestamptz',
  'date',
  'uuid',
  'jsonb',
];

export function TableStructureTab({
  dbName,
  schema,
  table,
  engineKind,
  onDropped,
  onChanged,
  onNotify,
}: TableStructureTabProps) {
  const [structure, setStructure] = useState<TableStructure | null>(null);
  const [loading, setLoading] = useState(true);

  // Table danger actions
  const [dropOpen, setDropOpen] = useState(false);
  const [truncateOpen, setTruncateOpen] = useState(false);
  const [truncateCascade, setTruncateCascade] = useState(false);
  const [busy, setBusy] = useState(false);

  // Add column modal
  const [addColOpen, setAddColOpen] = useState(false);
  const [colName, setColName] = useState('');
  const [colType, setColType] = useState('text');
  const [colNullable, setColNullable] = useState(true);
  const [colDefault, setColDefault] = useState('');

  // Edit column modal
  const [editCol, setEditCol] = useState<ColumnInfo | null>(null);
  const [editColName, setEditColName] = useState('');
  const [editColType, setEditColType] = useState('');
  const [editColNullable, setEditColNullable] = useState(true);
  const [editColDefault, setEditColDefault] = useState('');
  const [editDropDefault, setEditDropDefault] = useState(false);

  // Drop column / index / constraint confirm
  const [dropCol, setDropCol] = useState<ColumnInfo | null>(null);
  const [dropIdx, setDropIdx] = useState<IndexInfo | null>(null);
  const [dropCon, setDropCon] = useState<ConstraintInfo | null>(null);

  // Create index modal
  const [idxOpen, setIdxOpen] = useState(false);
  const [idxName, setIdxName] = useState('');
  const [idxCols, setIdxCols] = useState<string[]>([]);
  const [idxUnique, setIdxUnique] = useState(false);
  const [idxMethod, setIdxMethod] = useState('btree');

  // Add constraint modal
  const [conOpen, setConOpen] = useState(false);
  const [conType, setConType] = useState<'FOREIGN KEY' | 'UNIQUE' | 'CHECK'>('FOREIGN KEY');
  const [conName, setConName] = useState('');
  const [conCols, setConCols] = useState<string[]>([]);
  const [refSchema, setRefSchema] = useState(schema || 'public');
  const [refTable, setRefTable] = useState('');
  const [refColumns, setRefColumns] = useState('id');
  const [onDelete, setOnDelete] = useState('RESTRICT');
  const [onUpdate, setOnUpdate] = useState('CASCADE');
  const [checkExpr, setCheckExpr] = useState('');

  const loadStructure = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.tables.structure(dbName, schema, table);
      setStructure(res);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to load structure');
    } finally {
      setLoading(false);
    }
  }, [dbName, schema, table, onNotify]);

  useEffect(() => {
    void loadStructure();
  }, [loadStructure]);

  const refreshAll = async () => {
    await loadStructure();
    onChanged?.();
  };

  const handleDropTable = async () => {
    setBusy(true);
    try {
      await api.tables.drop(dbName, schema, table);
      onNotify('success', `Dropped table ${schema}.${table}`);
      setDropOpen(false);
      onDropped();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Drop table failed');
    } finally {
      setBusy(false);
    }
  };

  const handleTruncateTable = async () => {
    setBusy(true);
    try {
      await api.tables.truncate(dbName, schema, table, truncateCascade);
      onNotify('success', `Truncated table ${schema}.${table}`);
      setTruncateOpen(false);
      await refreshAll();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Truncate failed');
    } finally {
      setBusy(false);
    }
  };

  const handleAddColumn = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.tables.addColumn(dbName, schema, table, {
        name: colName.trim(),
        type: colType.trim(),
        nullable: colNullable,
        default: colDefault.trim(),
      });
      onNotify('success', `Added column "${colName.trim()}"`);
      setAddColOpen(false);
      setColName('');
      setColDefault('');
      await refreshAll();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Add column failed');
    } finally {
      setBusy(false);
    }
  };

  const openEditColumn = (c: ColumnInfo) => {
    setEditCol(c);
    setEditColName(c.name);
    setEditColType(c.formatType || c.dataType);
    setEditColNullable(c.isNullable);
    setEditColDefault(c.defaultValue ?? '');
    setEditDropDefault(false);
  };

  const handleEditColumn = async (e: FormEvent) => {
    e.preventDefault();
    if (!editCol) return;
    setBusy(true);
    try {
      const payload: {
        newName?: string;
        newType?: string;
        setNullable?: boolean;
        setDefault?: string;
        dropDefault?: boolean;
      } = {};
      if (editColName.trim() !== editCol.name) {
        payload.newName = editColName.trim();
      }
      if (editColType.trim() !== (editCol.formatType || editCol.dataType)) {
        payload.newType = editColType.trim();
      }
      if (editColNullable !== editCol.isNullable) {
        payload.setNullable = editColNullable;
      }
      if (editDropDefault) {
        payload.dropDefault = true;
      } else if (editColDefault.trim() !== (editCol.defaultValue ?? '')) {
        if (editColDefault.trim() === '') {
          payload.dropDefault = true;
        } else {
          payload.setDefault = editColDefault.trim();
        }
      }

      await api.tables.alterColumn(dbName, schema, table, editCol.name, payload);
      onNotify('success', `Updated column "${editCol.name}"`);
      setEditCol(null);
      await refreshAll();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Alter column failed');
    } finally {
      setBusy(false);
    }
  };

  const handleDropColumn = async () => {
    if (!dropCol) return;
    setBusy(true);
    try {
      await api.tables.dropColumn(dbName, schema, table, dropCol.name);
      onNotify('success', `Dropped column "${dropCol.name}"`);
      setDropCol(null);
      await refreshAll();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Drop column failed');
    } finally {
      setBusy(false);
    }
  };

  const toggleArrItem = (arr: string[], item: string) =>
    arr.includes(item) ? arr.filter((x) => x !== item) : [...arr, item];

  const handleCreateIndex = async (e: FormEvent) => {
    e.preventDefault();
    if (idxCols.length === 0) {
      onNotify('error', 'Select at least one column for the index');
      return;
    }
    setBusy(true);
    try {
      await api.tables.createIndex(dbName, schema, table, {
        name: idxName.trim(),
        columns: idxCols,
        unique: idxUnique,
        method: idxMethod,
      });
      onNotify('success', `Created index "${idxName.trim()}"`);
      setIdxOpen(false);
      await loadStructure();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Create index failed');
    } finally {
      setBusy(false);
    }
  };

  const handleDropIndex = async () => {
    if (!dropIdx) return;
    setBusy(true);
    try {
      await api.tables.dropIndex(dbName, schema, table, dropIdx.name);
      onNotify('success', `Dropped index "${dropIdx.name}"`);
      setDropIdx(null);
      await loadStructure();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Drop index failed');
    } finally {
      setBusy(false);
    }
  };

  const handleAddConstraint = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.tables.addConstraint(dbName, schema, table, {
        name: conName.trim(),
        type: conType,
        columns: conCols,
        refSchema: refSchema.trim(),
        refTable: refTable.trim(),
        refColumns: refColumns
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
        onDelete,
        onUpdate,
        checkExpr: checkExpr.trim(),
      });
      onNotify('success', `Added ${conType} constraint "${conName.trim()}"`);
      setConOpen(false);
      await loadStructure();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Add constraint failed');
    } finally {
      setBusy(false);
    }
  };

  const handleDropConstraint = async () => {
    if (!dropCon) return;
    setBusy(true);
    try {
      await api.tables.dropConstraint(dbName, schema, table, dropCon.name);
      onNotify('success', `Dropped constraint "${dropCon.name}"`);
      setDropCon(null);
      await loadStructure();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Drop constraint failed');
    } finally {
      setBusy(false);
    }
  };

  const handleMaintain = async (
    action: 'vacuum_analyze' | 'reindex' | 'refresh_matview'
  ) => {
    setBusy(true);
    try {
      const res = await api.tables.maintain(dbName, schema, table, action);
      onNotify('success', `${res.commandTag || action} (${res.durationMs}ms)`);
      await refreshAll();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Maintenance failed');
    } finally {
      setBusy(false);
    }
  };

  if (loading) {
    return (
      <div className="py-16 text-center text-xs text-slate-500">
        <Loader2 className="w-5 h-5 animate-spin mx-auto mb-2 text-indigo-500" />
        Loading table structure...
      </div>
    );
  }

  if (!structure) return null;

  const isView = structure.type === 'VIEW' || structure.type === 'MATERIALIZED VIEW';
  const isMatView = structure.type === 'MATERIALIZED VIEW';

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold font-mono">
            {structure.schema}.{structure.table}
          </h3>
          <p className="text-xs text-slate-500">
            {structure.type} &bull; {structure.columns.length} columns &bull;{' '}
            {structure.indexes.length} indexes &bull; {structure.constraints?.length ?? 0} constraints
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {isMatView && (
            <button
              type="button"
              disabled={busy}
              onClick={() => void handleMaintain('refresh_matview')}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium"
            >
              <RefreshCw className="w-3.5 h-3.5" />
              Refresh Materialized View
            </button>
          )}

          {!isView && (
            <>
              <button
                type="button"
                onClick={() => {
                  setColName('');
                  setColType('text');
                  setColNullable(true);
                  setColDefault('');
                  setAddColOpen(true);
                }}
                className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium"
              >
                <Plus className="w-3.5 h-3.5" />
                Add Column
              </button>

              <button
                type="button"
                onClick={() => {
                  const firstCol = structure.columns[0]?.name || 'col';
                  setIdxCols([firstCol]);
                  setIdxName(`idx_${table}_${firstCol}`);
                  setIdxUnique(false);
                  setIdxMethod('btree');
                  setIdxOpen(true);
                }}
                className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium"
              >
                <Sparkles className="w-3.5 h-3.5 text-amber-500" />
                Create Index
              </button>

              {engineKind !== 'sqlite' && (
                <button
                  type="button"
                  onClick={() => {
                    const firstCol = structure.columns[0]?.name || '';
                    setConType('FOREIGN KEY');
                    setConCols(firstCol ? [firstCol] : []);
                    setConName(`fk_${table}_${firstCol || 'col'}`);
                    setRefSchema(schema || 'public');
                    setRefTable('');
                    setRefColumns('id');
                    setCheckExpr('');
                    setConOpen(true);
                  }}
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium"
                >
                  <Shield className="w-3.5 h-3.5 text-cyan-500" />
                  Add Constraint
                </button>
              )}

              {engineKind === 'postgres' && (
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void handleMaintain('vacuum_analyze')}
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium"
                >
                  <RefreshCw className="w-3.5 h-3.5 text-emerald-500" />
                  Vacuum Analyze
                </button>
              )}

              <button
                type="button"
                onClick={() => {
                  setTruncateCascade(false);
                  setTruncateOpen(true);
                }}
                className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-amber-500/40 text-amber-600 dark:text-amber-400 hover:bg-amber-500/10 text-xs font-medium"
              >
                <Eraser className="w-3.5 h-3.5" />
                Truncate
              </button>

              <button
                type="button"
                onClick={() => setDropOpen(true)}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-red-500/40 text-red-500 hover:bg-red-500/10 text-xs font-medium"
              >
                <Trash2 className="w-3.5 h-3.5" />
                Drop Table
              </button>
            </>
          )}
        </div>
      </div>

      {/* Columns */}
      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
        <div className="px-4 py-2.5 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 text-xs font-semibold">
          Columns
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-slate-50/70 dark:bg-slate-900/50 text-slate-500 border-b border-slate-200 dark:border-slate-800">
              <tr>
                <th className="px-4 py-2">#</th>
                <th className="px-4 py-2">Name</th>
                <th className="px-4 py-2">Type</th>
                <th className="px-4 py-2">Nullable</th>
                <th className="px-4 py-2">Default</th>
                <th className="px-4 py-2">Keys &amp; Relations</th>
                {!isView && <th className="px-4 py-2 text-right">DDL</th>}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70">
              {structure.columns.map((c) => (
                <tr key={c.name} className="hover:bg-slate-50 dark:hover:bg-slate-800/30">
                  <td className="px-4 py-2.5 font-mono text-slate-400">{c.ordinalPos}</td>
                  <td className="px-4 py-2.5 font-mono font-semibold text-slate-900 dark:text-white">
                    {c.name}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-indigo-500">
                    {c.formatType}
                    {c.enumValues && c.enumValues.length > 0 && (
                      <div className="text-[10px] text-slate-400 mt-0.5">
                        ENUM ({c.enumValues.slice(0, 4).join(', ')}
                        {c.enumValues.length > 4 ? `, +${c.enumValues.length - 4}` : ''})
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-2.5">
                    {c.isNullable ? (
                      <span className="text-slate-400">YES</span>
                    ) : (
                      <span className="text-rose-500 font-semibold">NOT NULL</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-slate-500 max-w-xs truncate">
                    {c.defaultValue || '-'}
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex flex-wrap gap-1.5">
                      {c.isPrimaryKey && (
                        <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-600 dark:text-amber-300 text-[10px] font-semibold">
                          <Key className="w-2.5 h-2.5" /> PRIMARY KEY
                        </span>
                      )}
                      {c.foreignKey && (
                        <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-cyan-500/15 text-cyan-600 dark:text-cyan-300 text-[10px] font-mono">
                          <Link2 className="w-2.5 h-2.5" />
                          {c.foreignKey.refSchema}.{c.foreignKey.refTable}.{c.foreignKey.refColumn}
                        </span>
                      )}
                    </div>
                  </td>
                  {!isView && (
                    <td className="px-4 py-2.5 text-right whitespace-nowrap">
                      <div className="inline-flex items-center gap-1">
                        <button
                          type="button"
                          onClick={() => openEditColumn(c)}
                          title={`Edit column ${c.name}`}
                          className="p-1.5 rounded-lg text-slate-400 hover:text-indigo-500 hover:bg-indigo-500/10"
                        >
                          <Pencil className="w-3.5 h-3.5" />
                        </button>
                        {!c.isPrimaryKey && structure.columns.length > 1 && (
                          <button
                            type="button"
                            onClick={() => setDropCol(c)}
                            title={`Drop column ${c.name}`}
                            className="p-1.5 rounded-lg text-slate-400 hover:text-red-500 hover:bg-red-500/10"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        )}
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Constraints */}
      {structure.constraints && structure.constraints.length > 0 && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
          <div className="px-4 py-2.5 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 text-xs font-semibold">
            Constraints
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800/70">
            {structure.constraints.map((con) => (
              <div
                key={`${con.type}-${con.name}`}
                className="px-4 py-3 flex items-start justify-between gap-3"
              >
                <div className="space-y-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-semibold">{con.name}</span>
                    <span className="px-1.5 py-0.5 rounded bg-indigo-500/15 text-indigo-500 text-[10px] font-semibold">
                      {con.type}
                    </span>
                    {con.columns && (
                      <span className="text-[11px] font-mono text-slate-400">
                        ({con.columns})
                      </span>
                    )}
                  </div>
                  <p className="text-[11px] font-mono text-slate-500 break-all">
                    {con.definition}
                  </p>
                </div>
                {!isView && con.type !== 'PRIMARY KEY' && engineKind !== 'sqlite' && (
                  <button
                    type="button"
                    onClick={() => setDropCon(con)}
                    title={`Drop constraint ${con.name}`}
                    className="p-1.5 rounded-lg text-slate-400 hover:text-red-500 hover:bg-red-500/10 shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Indexes */}
      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
        <div className="px-4 py-2.5 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 text-xs font-semibold flex items-center justify-between">
          <span>Indexes</span>
          {!isView && (
            <button
              type="button"
              onClick={() => {
                const firstCol = structure.columns[0]?.name || 'col';
                setIdxCols([firstCol]);
                setIdxName(`idx_${table}_${firstCol}`);
                setIdxOpen(true);
              }}
              className="text-[11px] text-indigo-500 hover:text-indigo-400 font-medium"
            >
              + New Index
            </button>
          )}
        </div>
        {structure.indexes.length === 0 ? (
          <div className="px-4 py-6 text-center text-xs text-slate-400">
            No indexes defined on this table.
          </div>
        ) : (
          <div className="divide-y divide-slate-100 dark:divide-slate-800/70">
            {structure.indexes.map((idx) => (
              <div
                key={idx.name}
                className="px-4 py-3 flex items-start justify-between gap-3"
              >
                <div className="space-y-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-semibold">{idx.name}</span>
                    {idx.isPrimary && (
                      <span className="px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-500 text-[10px] font-semibold">
                        PRIMARY
                      </span>
                    )}
                    {idx.isUnique && !idx.isPrimary && (
                      <span className="px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-500 text-[10px] font-semibold">
                        UNIQUE
                      </span>
                    )}
                  </div>
                  <p className="text-[11px] font-mono text-slate-500 break-all">
                    {idx.definition}
                  </p>
                </div>
                {!isView && !idx.isPrimary && (
                  <button
                    type="button"
                    onClick={() => setDropIdx(idx)}
                    title={`Drop index ${idx.name}`}
                    className="p-1.5 rounded-lg text-slate-400 hover:text-red-500 hover:bg-red-500/10 shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Add Column Modal */}
      <Modal
        open={addColOpen}
        onClose={() => setAddColOpen(false)}
        title={`Add Column — ${schema}.${table}`}
      >
        <form onSubmit={handleAddColumn} className="space-y-3.5">
          <div>
            <label className="block text-xs font-medium mb-1">Column Name</label>
            <input
              type="text"
              required
              autoFocus
              value={colName}
              onChange={(e) => setColName(e.target.value)}
              placeholder="e.g. phone_number"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">SQL Data Type</label>
            <input
              type="text"
              required
              list="ddl-type-suggestions"
              value={colType}
              onChange={(e) => setColType(e.target.value)}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
            <datalist id="ddl-type-suggestions">
              {PG_TYPES.map((t) => (
                <option key={t} value={t} />
              ))}
            </datalist>
          </div>
          <label className="flex items-center gap-2 text-xs cursor-pointer">
            <input
              type="checkbox"
              checked={colNullable}
              onChange={(e) => setColNullable(e.target.checked)}
              className="rounded border-slate-300 text-indigo-600"
            />
            Allow NULL values
          </label>
          <div>
            <label className="block text-xs font-medium mb-1">
              Default Expression (optional)
            </label>
            <input
              type="text"
              value={colDefault}
              onChange={(e) => setColDefault(e.target.value)}
              placeholder="e.g. 'active', 0, now()"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setAddColOpen(false)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={busy}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 text-white text-xs font-semibold"
            >
              {busy && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Add Column
            </button>
          </div>
        </form>
      </Modal>

      {/* Edit Column Modal */}
      <Modal
        open={!!editCol}
        onClose={() => setEditCol(null)}
        title={`Alter Column — ${editCol?.name ?? ''}`}
        subtitle={`${schema}.${table}`}
      >
        <form onSubmit={handleEditColumn} className="space-y-3.5">
          <div>
            <label className="block text-xs font-medium mb-1">Column Name</label>
            <input
              type="text"
              required
              value={editColName}
              onChange={(e) => setEditColName(e.target.value)}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>
          {engineKind !== 'sqlite' && (
            <>
              <div>
                <label className="block text-xs font-medium mb-1">SQL Data Type</label>
                <input
                  type="text"
                  required
                  list="ddl-type-suggestions"
                  value={editColType}
                  onChange={(e) => setEditColType(e.target.value)}
                  className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
                />
              </div>
              <label className="flex items-center gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={editColNullable}
                  onChange={(e) => setEditColNullable(e.target.checked)}
                  className="rounded border-slate-300 text-indigo-600"
                />
                Allow NULL values
              </label>
              <div>
                <label className="block text-xs font-medium mb-1">Default Expression</label>
                <input
                  type="text"
                  disabled={editDropDefault}
                  value={editDropDefault ? '' : editColDefault}
                  onChange={(e) => setEditColDefault(e.target.value)}
                  placeholder="e.g. now(), 'pending', 0"
                  className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono disabled:opacity-50"
                />
                <label className="mt-1.5 flex items-center gap-2 text-[11px] text-slate-500 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={editDropDefault}
                    onChange={(e) => setEditDropDefault(e.target.checked)}
                    className="rounded border-slate-300 text-indigo-600"
                  />
                  Remove default constraint completely
                </label>
              </div>
            </>
          )}
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setEditCol(null)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={busy}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 text-white text-xs font-semibold"
            >
              {busy && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Save Changes
            </button>
          </div>
        </form>
      </Modal>

      {/* Create Index Modal */}
      <Modal
        open={idxOpen}
        onClose={() => setIdxOpen(false)}
        title={`Create Index — ${schema}.${table}`}
      >
        <form onSubmit={handleCreateIndex} className="space-y-3.5">
          <div>
            <label className="block text-xs font-medium mb-1">Index Name</label>
            <input
              type="text"
              required
              value={idxName}
              onChange={(e) => setIdxName(e.target.value)}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1.5">Columns (ordered)</label>
            <div className="flex flex-wrap gap-1.5">
              {structure.columns.map((c) => {
                const picked = idxCols.includes(c.name);
                return (
                  <button
                    key={c.name}
                    type="button"
                    onClick={() => setIdxCols((prev) => toggleArrItem(prev, c.name))}
                    className={`px-2.5 py-1 rounded-lg border text-xs font-mono transition-colors ${
                      picked
                        ? 'bg-indigo-600 border-indigo-600 text-white'
                        : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'
                    }`}
                  >
                    {c.name}
                  </button>
                );
              })}
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {engineKind === 'postgres' && (
              <div>
                <label className="block text-xs font-medium mb-1">Index Method</label>
                <select
                  value={idxMethod}
                  onChange={(e) => setIdxMethod(e.target.value)}
                  className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
                >
                  <option value="btree">btree</option>
                  <option value="hash">hash</option>
                  <option value="gin">gin (JSONB / Arrays)</option>
                  <option value="gist">gist</option>
                  <option value="brin">brin</option>
                </select>
              </div>
            )}
            <label className="flex items-center gap-2 text-xs cursor-pointer pt-5">
              <input
                type="checkbox"
                checked={idxUnique}
                onChange={(e) => setIdxUnique(e.target.checked)}
                className="rounded border-slate-300 text-indigo-600"
              />
              Unique Index
            </label>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setIdxOpen(false)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={busy}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 text-white text-xs font-semibold"
            >
              {busy && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Create Index
            </button>
          </div>
        </form>
      </Modal>

      {/* Add Constraint Modal */}
      <Modal
        open={conOpen}
        onClose={() => setConOpen(false)}
        title={`Add Constraint — ${schema}.${table}`}
      >
        <form onSubmit={handleAddConstraint} className="space-y-3.5">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium mb-1">Constraint Type</label>
              <select
                value={conType}
                onChange={(e) =>
                  setConType(e.target.value as 'FOREIGN KEY' | 'UNIQUE' | 'CHECK')
                }
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs"
              >
                <option value="FOREIGN KEY">FOREIGN KEY</option>
                <option value="UNIQUE">UNIQUE</option>
                <option value="CHECK">CHECK</option>
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">Constraint Name</label>
              <input
                type="text"
                required
                value={conName}
                onChange={(e) => setConName(e.target.value)}
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
              />
            </div>
          </div>

          {conType !== 'CHECK' && (
            <div>
              <label className="block text-xs font-medium mb-1.5">Local Column(s)</label>
              <div className="flex flex-wrap gap-1.5">
                {structure.columns.map((c) => {
                  const picked = conCols.includes(c.name);
                  return (
                    <button
                      key={c.name}
                      type="button"
                      onClick={() => setConCols((prev) => toggleArrItem(prev, c.name))}
                      className={`px-2.5 py-1 rounded-lg border text-xs font-mono transition-colors ${
                        picked
                          ? 'bg-indigo-600 border-indigo-600 text-white'
                          : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'
                      }`}
                    >
                      {c.name}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {conType === 'FOREIGN KEY' && (
            <div className="space-y-3 rounded-xl border border-slate-200 dark:border-slate-800 p-3">
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="block text-[11px] font-medium mb-1">Ref Schema</label>
                  <input
                    type="text"
                    value={refSchema}
                    onChange={(e) => setRefSchema(e.target.value)}
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
                  />
                </div>
                <div>
                  <label className="block text-[11px] font-medium mb-1">Ref Table</label>
                  <input
                    type="text"
                    required
                    value={refTable}
                    onChange={(e) => setRefTable(e.target.value)}
                    placeholder="users"
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
                  />
                </div>
                <div>
                  <label className="block text-[11px] font-medium mb-1">Ref Column(s)</label>
                  <input
                    type="text"
                    required
                    value={refColumns}
                    onChange={(e) => setRefColumns(e.target.value)}
                    placeholder="id"
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="block text-[11px] font-medium mb-1">ON DELETE</label>
                  <select
                    value={onDelete}
                    onChange={(e) => setOnDelete(e.target.value)}
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
                  >
                    <option value="RESTRICT">RESTRICT</option>
                    <option value="CASCADE">CASCADE</option>
                    <option value="SET NULL">SET NULL</option>
                    <option value="NO ACTION">NO ACTION</option>
                  </select>
                </div>
                <div>
                  <label className="block text-[11px] font-medium mb-1">ON UPDATE</label>
                  <select
                    value={onUpdate}
                    onChange={(e) => setOnUpdate(e.target.value)}
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
                  >
                    <option value="CASCADE">CASCADE</option>
                    <option value="RESTRICT">RESTRICT</option>
                    <option value="SET NULL">SET NULL</option>
                    <option value="NO ACTION">NO ACTION</option>
                  </select>
                </div>
              </div>
            </div>
          )}

          {conType === 'CHECK' && (
            <div>
              <label className="block text-xs font-medium mb-1">CHECK Expression</label>
              <input
                type="text"
                required
                value={checkExpr}
                onChange={(e) => setCheckExpr(e.target.value)}
                placeholder="e.g. amount >= 0"
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
              />
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setConOpen(false)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={busy}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 text-white text-xs font-semibold"
            >
              {busy && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Add Constraint
            </button>
          </div>
        </form>
      </Modal>

      {/* Custom HTML/CSS Confirm Dialogs (Zero Browser Popups) */}
      <ConfirmModal
        open={dropOpen}
        onClose={() => setDropOpen(false)}
        title={`Drop Table — ${schema}.${table}`}
        subtitle="This permanently deletes the table structure, indexes, and all rows."
        confirmTextMatch={table}
        confirmLabel="Drop Table"
        loading={busy}
        onConfirm={handleDropTable}
        message={
          <>
            You are about to permanently drop table{' '}
            <code className="font-mono font-semibold">
              {schema}.{table}
            </code>{' '}
            from <code className="font-mono font-semibold">{dbName}</code>. This cannot be undone.
          </>
        }
      />

      <ConfirmModal
        open={truncateOpen}
        onClose={() => setTruncateOpen(false)}
        title={`Truncate Table — ${schema}.${table}`}
        subtitle="Deletes every row in this table while preserving columns & indexes."
        confirmTextMatch={table}
        confirmLabel="Truncate All Rows"
        loading={busy}
        onConfirm={handleTruncateTable}
        message={
          <div className="space-y-2">
            <p>
              All data inside{' '}
              <code className="font-mono font-semibold">
                {schema}.{table}
              </code>{' '}
              will be cleared.
            </p>
            {engineKind === 'postgres' && (
              <label className="flex items-center gap-2 text-xs cursor-pointer font-medium">
                <input
                  type="checkbox"
                  checked={truncateCascade}
                  onChange={(e) => setTruncateCascade(e.target.checked)}
                  className="rounded border-slate-300 text-red-600"
                />
                CASCADE to referencing tables (and RESTART IDENTITY)
              </label>
            )}
          </div>
        }
      />

      <ConfirmModal
        open={!!dropCol}
        onClose={() => setDropCol(null)}
        title="Drop Column"
        subtitle={`${schema}.${table}.${dropCol?.name ?? ''}`}
        confirmTextMatch={dropCol?.name}
        confirmLabel="Drop Column"
        loading={busy}
        onConfirm={handleDropColumn}
        message={
          <>
            Dropping column <strong className="font-mono">{dropCol?.name}</strong> permanently removes all values stored in it.
          </>
        }
      />

      <ConfirmModal
        open={!!dropIdx}
        onClose={() => setDropIdx(null)}
        title="Drop Index"
        subtitle={`${schema}.${table}`}
        confirmTextMatch={dropIdx?.name}
        confirmLabel="Drop Index"
        loading={busy}
        onConfirm={handleDropIndex}
        message={
          <>
            Drop index <strong className="font-mono">{dropIdx?.name}</strong>? Queries may slow down until recreated.
          </>
        }
      />

      <ConfirmModal
        open={!!dropCon}
        onClose={() => setDropCon(null)}
        title="Drop Constraint"
        subtitle={`${schema}.${table}`}
        confirmTextMatch={dropCon?.name}
        confirmLabel="Drop Constraint"
        loading={busy}
        onConfirm={handleDropConstraint}
        message={
          <>
            Drop {dropCon?.type} constraint <strong className="font-mono">{dropCon?.name}</strong>?
          </>
        }
      />
    </div>
  );
}

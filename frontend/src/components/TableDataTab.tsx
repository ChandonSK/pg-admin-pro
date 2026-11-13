import { useCallback, useEffect, useState, type FormEvent } from 'react';
import {
  ArrowDown,
  ArrowUp,
  Check,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Download,
  Edit3,
  ExternalLink,
  Filter,
  Key,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  Upload,
  X,
} from 'lucide-react';
import { api } from '../api/client';
import type { ColumnInfo, FilterCond, QueryRowsResp } from '../types';
import { ImportRowsModal } from './ImportRowsModal';
import { Modal } from './Modal';
import { RowEditorModal } from './RowEditorModal';

interface TableDataTabProps {
  dbName: string;
  schema: string;
  table: string;
  tableType: string;
  initialFilter?: FilterCond | null;
  onJumpForeignKey?: (refSchema: string, refTable: string, refColumn: string, value: string) => void;
  onNotify: (kind: 'success' | 'error' | 'info', msg: string) => void;
}

function renderCellValue(val: unknown): { text: string; isNull: boolean } {
  if (val === null || val === undefined) {
    return { text: 'NULL', isNull: true };
  }
  if (typeof val === 'object') {
    try {
      return { text: JSON.stringify(val), isNull: false };
    } catch {
      return { text: String(val), isNull: false };
    }
  }
  if (typeof val === 'boolean') {
    return { text: val ? 'true' : 'false', isNull: false };
  }
  return { text: String(val), isNull: false };
}

function parseInlineValue(col: ColumnInfo, raw: string): unknown {
  const ft = col.formatType.toLowerCase();
  if (ft === 'boolean' || ft === 'bool') {
    return raw === 'true';
  }
  if (ft === 'json' || ft === 'jsonb') {
    try {
      return JSON.parse(raw);
    } catch {
      return raw;
    }
  }
  return raw;
}

export function TableDataTab({
  dbName,
  schema,
  table,
  tableType,
  initialFilter,
  onJumpForeignKey,
  onNotify,
}: TableDataTabProps) {
  const [data, setData] = useState<QueryRowsResp | null>(null);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [sortBy, setSortBy] = useState('');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
  const [searchInput, setSearchInput] = useState('');
  const [appliedSearch, setAppliedSearch] = useState('');

  // Quick filter bar
  const [filterOpen, setFilterOpen] = useState(false);
  const [filterCol, setFilterCol] = useState('');
  const [filterOp, setFilterOp] = useState('=');
  const [filterVal, setFilterVal] = useState('');
  const [activeFilters, setActiveFilters] = useState<FilterCond[]>([]);

  // Row selection & modals
  const [selectedIndices, setSelectedIndices] = useState<Set<number>>(new Set());
  const [insertOpen, setInsertOpen] = useState(false);
  const [editIndex, setEditIndex] = useState<number | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
  const [pendingDeleteIndices, setPendingDeleteIndices] = useState<number[]>([]);
  const [deleting, setDeleting] = useState(false);

  // Inline cell editor state
  const [editingCell, setEditingCell] = useState<{
    rowIdx: number;
    col: ColumnInfo;
    draft: string;
    saving: boolean;
  } | null>(null);

  const isReadOnlyView = tableType === 'VIEW' || tableType === 'MATERIALIZED VIEW';

  // Reset state when table or initial FK jump filter switches
  useEffect(() => {
    setPage(1);
    setSortBy('');
    setSortDir('asc');
    setSearchInput('');
    setAppliedSearch('');
    setActiveFilters(initialFilter ? [initialFilter] : []);
    setSelectedIndices(new Set());
    setFilterCol('');
    setEditingCell(null);
  }, [dbName, schema, table, initialFilter]);

  const fetchRows = useCallback(async () => {
    setLoading(true);
    try {
      const resp = await api.tables.queryRows(dbName, schema, table, {
        page,
        pageSize,
        sortBy: sortBy || undefined,
        sortDir,
        search: appliedSearch || undefined,
        filters: activeFilters,
      });
      setData(resp);
      if (resp.columns.length > 0 && !filterCol) {
        setFilterCol(resp.columns[0].name);
      }
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to load table rows');
    } finally {
      setLoading(false);
    }
  }, [dbName, schema, table, page, pageSize, sortBy, sortDir, appliedSearch, activeFilters, filterCol, onNotify]);

  useEffect(() => {
    void fetchRows();
  }, [fetchRows]);

  const handleSort = (colName: string) => {
    if (sortBy === colName) {
      setSortDir((prev) => (prev === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortBy(colName);
      setSortDir('asc');
    }
    setPage(1);
  };

  const handleSearchSubmit = (e: FormEvent) => {
    e.preventDefault();
    setAppliedSearch(searchInput.trim());
    setPage(1);
  };

  const handleAddFilter = (e: FormEvent) => {
    e.preventDefault();
    if (!filterCol) return;
    setActiveFilters((prev) => [
      ...prev,
      { column: filterCol, operator: filterOp, value: filterVal },
    ]);
    setFilterVal('');
    setPage(1);
  };

  const removeFilter = (idx: number) => {
    setActiveFilters((prev) => prev.filter((_, i) => i !== idx));
    setPage(1);
  };

  const toggleSelectAll = () => {
    if (!data) return;
    if (selectedIndices.size === data.rows.length) {
      setSelectedIndices(new Set());
    } else {
      setSelectedIndices(new Set(data.rows.map((_, i) => i)));
    }
  };

  const toggleRow = (idx: number) => {
    setSelectedIndices((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx);
      else next.add(idx);
      return next;
    });
  };

  const requestDelete = (indices: number[]) => {
    setPendingDeleteIndices(indices);
    setConfirmDeleteOpen(true);
  };

  const handleConfirmDelete = async () => {
    if (!data || pendingDeleteIndices.length === 0) return;
    setDeleting(true);
    try {
      const locators = pendingDeleteIndices.map((idx) => data.rowLocators[idx]);
      const res = await api.tables.deleteRows(dbName, schema, table, locators);
      onNotify('success', `Deleted ${res.deleted} row(s)`);
      setConfirmDeleteOpen(false);
      setPendingDeleteIndices([]);
      await fetchRows();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Delete failed');
    } finally {
      setDeleting(false);
    }
  };

  const handleExport = async (format: 'csv' | 'json' | 'sql', scope: 'page' | 'all') => {
    setExportOpen(false);
    setExporting(true);
    try {
      await api.tables.exportDownload(dbName, schema, table, {
        format,
        scope,
        page,
        pageSize,
        sortBy: sortBy || undefined,
        sortDir,
        search: appliedSearch || undefined,
        filters: activeFilters,
      });
      onNotify('success', `Exported ${scope} rows as ${format.toUpperCase()}`);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Export failed');
    } finally {
      setExporting(false);
    }
  };

  const totalPages = data ? Math.max(1, Math.ceil(data.totalCount / pageSize)) : 1;
  const canMutateRows =
    !isReadOnlyView && Boolean(data && (data.primaryKeys.length > 0 || data.usesRowIdFallback));

  const startInlineEdit = (rowIdx: number, col: ColumnInfo, currentVal: unknown) => {
    if (!canMutateRows || col.isGenerated) return;
    const cell = renderCellValue(currentVal);
    setEditingCell({
      rowIdx,
      col,
      draft: cell.isNull
        ? col.enumValues?.[0] || (col.formatType.toLowerCase().startsWith('bool') ? 'false' : '')
        : cell.text,
      saving: false,
    });
  };

  const commitInlineEdit = async (setNull = false) => {
    if (!editingCell || !data) return;
    const { rowIdx, col, draft } = editingCell;
    const locator = data.rowLocators[rowIdx];
    const nextValue = setNull ? null : parseInlineValue(col, draft);

    setEditingCell((prev) => (prev ? { ...prev, saving: true } : null));
    try {
      await api.tables.updateRow(dbName, schema, table, locator, {
        [col.name]: nextValue,
      });
      setEditingCell(null);
      onNotify('success', `Updated ${col.name}`);
      await fetchRows();
    } catch (err) {
      setEditingCell((prev) => (prev ? { ...prev, saving: false } : null));
      onNotify('error', err instanceof Error ? err.message : 'Inline update failed');
    }
  };

  const selectedFilterColumnMeta = data?.columns.find((c) => c.name === filterCol);

  return (
    <div className="flex flex-col h-full min-h-0 space-y-3">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <form onSubmit={handleSearchSubmit} className="relative">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-2.5 top-2" />
            <input
              type="text"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="Quick search..."
              className="pl-8 pr-7 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs w-44 focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
            {(searchInput || appliedSearch) && (
              <button
                type="button"
                onClick={() => {
                  setSearchInput('');
                  setAppliedSearch('');
                  setPage(1);
                }}
                aria-label="Clear search"
                className="absolute right-2 top-2 text-slate-400 hover:text-slate-200"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            )}
          </form>

          <button
            type="button"
            onClick={() => setFilterOpen((v) => !v)}
            className={`inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border text-xs font-medium transition-colors ${
              filterOpen || activeFilters.length > 0
                ? 'border-indigo-500 bg-indigo-500/10 text-indigo-500'
                : 'border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800'
            }`}
          >
            <Filter className="w-3.5 h-3.5" />
            Filter {activeFilters.length > 0 ? `(${activeFilters.length})` : ''}
          </button>

          <button
            type="button"
            onClick={fetchRows}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>

          {/* Export Dropdown */}
          <div className="relative">
            <button
              type="button"
              disabled={exporting}
              onClick={() => setExportOpen((v) => !v)}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium"
            >
              {exporting ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
              ) : (
                <Download className="w-3.5 h-3.5" />
              )}
              Export
              <ChevronDown className="w-3 h-3 text-slate-400" />
            </button>
            {exportOpen && (
              <div className="absolute left-0 mt-1.5 w-52 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-xl py-1.5 z-30 text-xs">
                <div className="px-3 py-1 text-[10px] font-semibold uppercase text-slate-400">
                  Current Page ({data?.rows.length ?? 0} rows)
                </div>
                {(['csv', 'json', 'sql'] as const).map((fmt) => (
                  <button
                    key={`page-${fmt}`}
                    type="button"
                    onClick={() => void handleExport(fmt, 'page')}
                    className="w-full text-left px-3 py-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 flex items-center justify-between"
                  >
                    <span>Page as {fmt.toUpperCase()}</span>
                  </button>
                ))}
                <div className="my-1 border-t border-slate-100 dark:border-slate-800" />
                <div className="px-3 py-1 text-[10px] font-semibold uppercase text-slate-400">
                  All Matching ({data?.totalCount ?? 0} rows)
                </div>
                {(['csv', 'json', 'sql'] as const).map((fmt) => (
                  <button
                    key={`all-${fmt}`}
                    type="button"
                    onClick={() => void handleExport(fmt, 'all')}
                    className="w-full text-left px-3 py-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 flex items-center justify-between"
                  >
                    <span>All rows as {fmt.toUpperCase()}</span>
                  </button>
                ))}
              </div>
            )}
          </div>

          {!isReadOnlyView && (
            <button
              type="button"
              onClick={() => setImportOpen(true)}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium"
            >
              <Upload className="w-3.5 h-3.5" />
              Import
            </button>
          )}
        </div>

        <div className="flex items-center gap-2">
          {selectedIndices.size > 0 && canMutateRows && (
            <button
              type="button"
              onClick={() => requestDelete(Array.from(selectedIndices))}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white text-xs font-semibold"
            >
              <Trash2 className="w-3.5 h-3.5" />
              Delete Selected ({selectedIndices.size})
            </button>
          )}

          {!isReadOnlyView && (
            <button
              type="button"
              onClick={() => setInsertOpen(true)}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-sm"
            >
              <Plus className="w-3.5 h-3.5" />
              Insert Row
            </button>
          )}
        </div>
      </div>

      {/* Filter Builder Bar */}
      {(filterOpen || activeFilters.length > 0) && data && (
        <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-900/70 p-3 space-y-2.5">
          {filterOpen && (
            <form onSubmit={handleAddFilter} className="flex flex-wrap items-center gap-2">
              <select
                value={filterCol}
                onChange={(e) => {
                  setFilterCol(e.target.value);
                  setFilterVal('');
                }}
                className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
              >
                {data.columns.map((c) => (
                  <option key={c.name} value={c.name}>
                    {c.name}
                  </option>
                ))}
              </select>
              <select
                value={filterOp}
                onChange={(e) => setFilterOp(e.target.value)}
                className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono"
              >
                <option value="=">=</option>
                <option value="!=">!=</option>
                <option value="ilike">ILIKE (contains)</option>
                <option value=">">&gt;</option>
                <option value=">=">&gt;=</option>
                <option value="<">&lt;</option>
                <option value="<=">&lt;=</option>
                <option value="is_null">IS NULL</option>
                <option value="is_not_null">IS NOT NULL</option>
              </select>
              {filterOp !== 'is_null' && filterOp !== 'is_not_null' && (
                selectedFilterColumnMeta?.enumValues && selectedFilterColumnMeta.enumValues.length > 0 && (filterOp === '=' || filterOp === '!=') ? (
                  <select
                    value={filterVal}
                    onChange={(e) => setFilterVal(e.target.value)}
                    className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono w-48"
                  >
                    <option value="">-- pick enum value --</option>
                    {selectedFilterColumnMeta.enumValues.map((ev) => (
                      <option key={ev} value={ev}>
                        {ev}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input
                    type="text"
                    value={filterVal}
                    onChange={(e) => setFilterVal(e.target.value)}
                    placeholder="Value..."
                    className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-2.5 py-1.5 text-xs font-mono w-48"
                  />
                )
              )}
              <button
                type="submit"
                className="px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold"
              >
                Apply Filter
              </button>
            </form>
          )}

          {activeFilters.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {activeFilters.map((f, i) => (
                <span
                  key={`${f.column}-${i}`}
                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-indigo-500/15 border border-indigo-500/30 text-indigo-600 dark:text-indigo-300 text-[11px] font-mono"
                >
                  {f.column} {f.operator} {f.operator.includes('null') ? '' : `"${f.value}"`}
                  <button type="button" onClick={() => removeFilter(i)}>
                    <X className="w-3 h-3" />
                  </button>
                </span>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Data Grid */}
      <div className="flex-1 min-h-[320px] rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden flex flex-col">
        <div className="flex-1 overflow-auto">
          <table className="w-full text-left text-xs border-collapse">
            <thead className="sticky top-0 z-10 bg-slate-100 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">
              <tr>
                {canMutateRows && (
                  <th className="w-10 px-3 py-2.5 text-center">
                    <input
                      type="checkbox"
                      aria-label="Select all rows"
                      checked={Boolean(data && data.rows.length > 0 && selectedIndices.size === data.rows.length)}
                      onChange={toggleSelectAll}
                    />
                  </th>
                )}
                {data?.columns.map((col) => {
                  const isSorted = sortBy === col.name;
                  return (
                    <th
                      key={col.name}
                      onClick={() => handleSort(col.name)}
                      className="px-3 py-2 font-semibold cursor-pointer select-none hover:bg-slate-200/60 dark:hover:bg-slate-800/70 whitespace-nowrap"
                    >
                      <div className="flex items-center gap-1">
                        {col.isPrimaryKey && (
                          <Key className="w-3 h-3 text-amber-500 shrink-0" />
                        )}
                        <span className="font-mono text-slate-900 dark:text-slate-100">
                          {col.name}
                        </span>
                        {isSorted &&
                          (sortDir === 'asc' ? (
                            <ArrowUp className="w-3 h-3 text-indigo-500" />
                          ) : (
                            <ArrowDown className="w-3 h-3 text-indigo-500" />
                          ))}
                      </div>
                      <div className="text-[10px] font-mono font-normal text-slate-400">
                        {col.formatType}
                        {col.foreignKey ? ` → ${col.foreignKey.refTable}` : ''}
                      </div>
                    </th>
                  );
                })}
                {canMutateRows && (
                  <th className="sticky right-0 bg-slate-100 dark:bg-slate-900 px-3 py-2.5 text-right font-semibold">
                    Actions
                  </th>
                )}
              </tr>
            </thead>

            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70 font-mono">
              {data?.rows.map((row, idx) => (
                <tr
                  key={idx}
                  className={`hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors ${
                    selectedIndices.has(idx) ? 'bg-indigo-500/10' : ''
                  }`}
                >
                  {canMutateRows && (
                    <td className="px-3 py-2 text-center">
                      <input
                        type="checkbox"
                        aria-label={`Select row ${idx + 1}`}
                        checked={selectedIndices.has(idx)}
                        onChange={() => toggleRow(idx)}
                      />
                    </td>
                  )}
                  {data.columns.map((col) => {
                    const cell = renderCellValue(row[col.name]);
                    const isInline =
                      editingCell?.rowIdx === idx && editingCell.col.name === col.name;
                    const ft = col.formatType.toLowerCase();
                    const isBool = ft === 'boolean' || ft === 'bool';
                    const hasEnum = Boolean(col.enumValues && col.enumValues.length > 0);

                    if (isInline && editingCell) {
                      return (
                        <td key={col.name} className="px-2 py-1 min-w-[180px] bg-indigo-500/5">
                          <div className="flex items-center gap-1">
                            {isBool ? (
                              <select
                                autoFocus
                                value={editingCell.draft === 'true' ? 'true' : 'false'}
                                onChange={(e) =>
                                  setEditingCell({ ...editingCell, draft: e.target.value })
                                }
                                onKeyDown={(e) => {
                                  if (e.key === 'Enter') void commitInlineEdit(false);
                                  if (e.key === 'Escape') setEditingCell(null);
                                }}
                                className="flex-1 rounded border border-indigo-500 bg-white dark:bg-slate-950 px-2 py-1 text-xs font-mono"
                              >
                                <option value="true">true</option>
                                <option value="false">false</option>
                              </select>
                            ) : hasEnum ? (
                              <select
                                autoFocus
                                value={editingCell.draft}
                                onChange={(e) =>
                                  setEditingCell({ ...editingCell, draft: e.target.value })
                                }
                                onKeyDown={(e) => {
                                  if (e.key === 'Enter') void commitInlineEdit(false);
                                  if (e.key === 'Escape') setEditingCell(null);
                                }}
                                className="flex-1 rounded border border-indigo-500 bg-white dark:bg-slate-950 px-2 py-1 text-xs font-mono"
                              >
                                {col.enumValues!.map((ev) => (
                                  <option key={ev} value={ev}>
                                    {ev}
                                  </option>
                                ))}
                              </select>
                            ) : (
                              <input
                                type="text"
                                autoFocus
                                value={editingCell.draft}
                                onChange={(e) =>
                                  setEditingCell({ ...editingCell, draft: e.target.value })
                                }
                                onKeyDown={(e) => {
                                  if (e.key === 'Enter') {
                                    e.preventDefault();
                                    void commitInlineEdit(false);
                                  } else if (e.key === 'Escape') {
                                    setEditingCell(null);
                                  }
                                }}
                                className="flex-1 rounded border border-indigo-500 bg-white dark:bg-slate-950 px-2 py-1 text-xs font-mono focus:outline-none"
                              />
                            )}
                            <button
                              type="button"
                              disabled={editingCell.saving}
                              onClick={() => void commitInlineEdit(false)}
                              title="Save cell (Enter)"
                              className="p-1 rounded bg-indigo-600 text-white hover:bg-indigo-500"
                            >
                              {editingCell.saving ? (
                                <Loader2 className="w-3 h-3 animate-spin" />
                              ) : (
                                <Check className="w-3 h-3" />
                              )}
                            </button>
                            {col.isNullable && (
                              <button
                                type="button"
                                disabled={editingCell.saving}
                                onClick={() => void commitInlineEdit(true)}
                                title="Set NULL"
                                className="px-1.5 py-0.5 rounded bg-slate-200 dark:bg-slate-700 text-[10px] hover:bg-slate-300"
                              >
                                NULL
                              </button>
                            )}
                            <button
                              type="button"
                              onClick={() => setEditingCell(null)}
                              title="Cancel (Esc)"
                              className="p-1 rounded text-slate-400 hover:text-slate-200"
                            >
                              <X className="w-3 h-3" />
                            </button>
                          </div>
                        </td>
                      );
                    }

                    return (
                      <td
                        key={col.name}
                        title={canMutateRows && !col.isGenerated ? `${cell.text} (Double-click to inline edit)` : cell.text}
                        onDoubleClick={() => startInlineEdit(idx, col, row[col.name])}
                        className="px-3 py-2 max-w-[260px] truncate whitespace-nowrap cursor-default"
                      >
                        {cell.isNull ? (
                          <span className="px-1.5 py-0.5 rounded bg-slate-200/70 dark:bg-slate-800 text-slate-400 italic text-[10px]">
                            NULL
                          </span>
                        ) : col.foreignKey && onJumpForeignKey ? (
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              onJumpForeignKey(
                                col.foreignKey!.refSchema,
                                col.foreignKey!.refTable,
                                col.foreignKey!.refColumn,
                                cell.text
                              );
                            }}
                            title={`Jump to ${col.foreignKey.refSchema}.${col.foreignKey.refTable} where ${col.foreignKey.refColumn} = ${cell.text}`}
                            className="inline-flex items-center gap-1 text-cyan-600 dark:text-cyan-400 hover:underline font-medium"
                          >
                            <span className="truncate max-w-[210px]">{cell.text}</span>
                            <ExternalLink className="w-2.5 h-2.5 shrink-0" />
                          </button>
                        ) : (
                          <span className="text-slate-800 dark:text-slate-200">
                            {cell.text}
                          </span>
                        )}
                      </td>
                    );
                  })}
                  {canMutateRows && (
                    <td className="sticky right-0 bg-white/95 dark:bg-slate-900/95 px-3 py-1.5 text-right whitespace-nowrap">
                      <div className="inline-flex items-center gap-1">
                        <button
                          type="button"
                          onClick={() => setEditIndex(idx)}
                          title="Edit full row in modal"
                          className="p-1.5 rounded-lg text-slate-500 hover:text-indigo-500 hover:bg-indigo-500/10"
                        >
                          <Edit3 className="w-3.5 h-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => requestDelete([idx])}
                          title="Delete row"
                          className="p-1.5 rounded-lg text-slate-500 hover:text-red-500 hover:bg-red-500/10"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>
                  )}
                </tr>
              ))}
              {!loading && data && data.rows.length === 0 && (
                <tr>
                  <td
                    colSpan={(data.columns.length || 1) + 2}
                    className="px-4 py-12 text-center font-sans text-slate-500"
                  >
                    No rows match the current filter.
                  </td>
                </tr>
              )}
              {loading && !data && (
                <tr>
                  <td colSpan={6} className="px-4 py-12 text-center font-sans text-slate-500">
                    <Loader2 className="w-5 h-5 animate-spin mx-auto mb-2 text-indigo-500" />
                    Loading rows...
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination Footer */}
        {data && (
          <div className="px-4 py-2.5 border-t border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-900/90 flex flex-wrap items-center justify-between gap-2 text-xs">
            <div className="text-slate-500 dark:text-slate-400">
              Total: <strong className="text-slate-900 dark:text-white">{data.totalCount.toLocaleString()}</strong> rows &bull;{' '}
              <span className="font-mono">{data.durationMs}ms</span>
              {canMutateRows && (
                <span className="hidden sm:inline ml-2 text-[11px] text-slate-400">
                  (Double-click any cell to inline edit)
                </span>
              )}
            </div>

            <div className="flex items-center gap-2">
              <select
                value={pageSize}
                onChange={(e) => {
                  setPageSize(Number(e.target.value));
                  setPage(1);
                }}
                aria-label="Rows per page"
                className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-950 px-2 py-1 text-xs"
              >
                {[25, 50, 100, 250].map((size) => (
                  <option key={size} value={size}>
                    {size} / page
                  </option>
                ))}
              </select>

              <button
                type="button"
                disabled={page <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className="p-1.5 rounded-lg border border-slate-200 dark:border-slate-700 disabled:opacity-40 hover:bg-slate-100 dark:hover:bg-slate-800"
                aria-label="Previous page"
              >
                <ChevronLeft className="w-3.5 h-3.5" />
              </button>
              <span className="font-mono text-xs px-1">
                {page} / {totalPages}
              </span>
              <button
                type="button"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                className="p-1.5 rounded-lg border border-slate-200 dark:border-slate-700 disabled:opacity-40 hover:bg-slate-100 dark:hover:bg-slate-800"
                aria-label="Next page"
              >
                <ChevronRight className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Insert Row Modal */}
      {data && (
        <RowEditorModal
          open={insertOpen}
          mode="insert"
          tableName={`${schema}.${table}`}
          columns={data.columns}
          onClose={() => setInsertOpen(false)}
          onSubmit={async (values) => {
            await api.tables.insertRow(dbName, schema, table, values);
            onNotify('success', 'New row inserted');
            await fetchRows();
          }}
        />
      )}

      {/* Edit Row Modal */}
      {data && editIndex !== null && data.rows[editIndex] && (
        <RowEditorModal
          open={true}
          mode="edit"
          tableName={`${schema}.${table}`}
          columns={data.columns}
          initialRow={data.rows[editIndex]}
          onClose={() => setEditIndex(null)}
          onSubmit={async (updates) => {
            if (Object.keys(updates).length === 0) {
              onNotify('info', 'No fields changed');
              return;
            }
            await api.tables.updateRow(
              dbName,
              schema,
              table,
              data.rowLocators[editIndex],
              updates
            );
            onNotify('success', 'Row updated');
            await fetchRows();
          }}
        />
      )}

      {/* Import CSV / JSON Modal */}
      {data && (
        <ImportRowsModal
          open={importOpen}
          dbName={dbName}
          schema={schema}
          table={table}
          columns={data.columns}
          onClose={() => setImportOpen(false)}
          onImported={fetchRows}
          onNotify={onNotify}
        />
      )}

      {/* Confirm Delete Modal */}
      <Modal
        open={confirmDeleteOpen}
        onClose={() => setConfirmDeleteOpen(false)}
        title="Delete Row(s)"
        subtitle="This operation executes a parameterized DELETE and cannot be undone."
      >
        <div className="space-y-4">
          <p className="text-xs text-slate-600 dark:text-slate-300">
            Are you sure you want to permanently delete{' '}
            <strong>{pendingDeleteIndices.length}</strong> selected row(s) from{' '}
            <code className="font-mono text-indigo-500">
              {schema}.{table}
            </code>
            ?
          </p>
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setConfirmDeleteOpen(false)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={deleting}
              onClick={handleConfirmDelete}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-red-600 hover:bg-red-500 text-white text-xs font-semibold"
            >
              {deleting && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Delete Permanently
            </button>
          </div>
        </div>
      </Modal>
    </div>
  );
}

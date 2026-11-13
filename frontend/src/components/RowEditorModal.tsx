import { useEffect, useState, type FormEvent } from 'react';
import { Key, Loader2 } from 'lucide-react';
import type { ColumnInfo } from '../types';
import { Modal } from './Modal';

interface RowEditorModalProps {
  open: boolean;
  mode: 'insert' | 'edit';
  tableName: string;
  columns: ColumnInfo[];
  initialRow?: Record<string, unknown>;
  onClose: () => void;
  onSubmit: (values: Record<string, unknown>) => Promise<void>;
}

interface FieldDraft {
  value: string;
  isNull: boolean;
  useDefault: boolean;
}

function formatInitialValue(val: unknown): string {
  if (val === null || val === undefined) return '';
  if (typeof val === 'object') {
    try {
      return JSON.stringify(val, null, 2);
    } catch {
      return String(val);
    }
  }
  return String(val);
}

function parseFieldForSubmit(col: ColumnInfo, raw: string): unknown {
  const ft = col.formatType.toLowerCase();
  if (ft === 'boolean' || ft === 'bool') {
    if (raw === 'true') return true;
    if (raw === 'false') return false;
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

export function RowEditorModal({
  open,
  mode,
  tableName,
  columns,
  initialRow,
  onClose,
  onSubmit,
}: RowEditorModalProps) {
  const [drafts, setDrafts] = useState<Record<string, FieldDraft>>({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setError('');
    const next: Record<string, FieldDraft> = {};
    for (const col of columns) {
      if (mode === 'insert') {
        const autoDefault = col.isIdentity || col.hasDefault || col.isGenerated;
        next[col.name] = {
          value: col.formatType.toLowerCase().startsWith('bool')
            ? 'false'
            : col.enumValues && col.enumValues.length > 0
            ? col.enumValues[0]
            : '',
          isNull: false,
          useDefault: autoDefault,
        };
      } else {
        const val = initialRow?.[col.name];
        next[col.name] = {
          value: formatInitialValue(val),
          isNull: val === null || val === undefined,
          useDefault: false,
        };
      }
    }
    setDrafts(next);
  }, [open, mode, columns, initialRow]);

  const updateDraft = (colName: string, patch: Partial<FieldDraft>) => {
    setDrafts((prev) => ({
      ...prev,
      [colName]: { ...prev[colName], ...patch },
    }));
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setSaving(true);
    try {
      const payload: Record<string, unknown> = {};
      for (const col of columns) {
        if (col.isGenerated) continue;
        const d = drafts[col.name];
        if (!d) continue;

        if (mode === 'insert') {
          if (d.useDefault) continue;
          payload[col.name] = d.isNull ? null : parseFieldForSubmit(col, d.value);
        } else {
          const origVal = initialRow?.[col.name];
          const origNull = origVal === null || origVal === undefined;
          const origStr = formatInitialValue(origVal);
          if (d.isNull && origNull) continue;
          if (!d.isNull && !origNull && d.value === origStr) continue;
          payload[col.name] = d.isNull ? null : parseFieldForSubmit(col, d.value);
        }
      }
      await onSubmit(payload);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={mode === 'insert' ? `Insert Row into ${tableName}` : `Edit Row in ${tableName}`}
      subtitle={
        mode === 'insert'
          ? 'Columns with DB defaults/auto-increment are skipped automatically unless unchecked.'
          : 'Only modified fields will be sent in the UPDATE statement.'
      }
      maxWidthClass="max-w-2xl"
    >
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-3">
          {columns.map((col) => {
            const d = drafts[col.name] || { value: '', isNull: false, useDefault: false };
            const ft = col.formatType.toLowerCase();
            const isBool = ft === 'boolean' || ft === 'bool';
            const enumVals = col.enumValues && col.enumValues.length > 0 ? col.enumValues : null;
            const isMultiline =
              !enumVals &&
              (ft === 'text' || ft === 'json' || ft === 'jsonb' || d.value.length > 80 || d.value.includes('\n'));

            return (
              <div
                key={col.name}
                className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50/70 dark:bg-slate-950/60 p-3 space-y-2"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-semibold text-slate-900 dark:text-white">
                      {col.name}
                    </span>
                    <span className="text-[11px] font-mono text-indigo-500">
                      {col.formatType}
                    </span>
                    {col.isPrimaryKey && (
                      <span className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-600 dark:text-amber-300 text-[10px] font-semibold">
                        <Key className="w-2.5 h-2.5" /> PK
                      </span>
                    )}
                    {!col.isNullable && (
                      <span className="text-[10px] text-rose-500 font-semibold">NOT NULL</span>
                    )}
                  </div>

                  <div className="flex items-center gap-3 text-[11px]">
                    {mode === 'insert' && (col.hasDefault || col.isIdentity) && !col.isGenerated && (
                      <label className="inline-flex items-center gap-1.5 text-slate-600 dark:text-slate-300 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={d.useDefault}
                          onChange={(e) =>
                            updateDraft(col.name, {
                              useDefault: e.target.checked,
                              isNull: false,
                            })
                          }
                        />
                        Use Default
                      </label>
                    )}

                    {col.isNullable && !col.isGenerated && (
                      <label className="inline-flex items-center gap-1.5 text-slate-600 dark:text-slate-300 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={d.isNull}
                          onChange={(e) =>
                            updateDraft(col.name, {
                              isNull: e.target.checked,
                              useDefault: false,
                            })
                          }
                        />
                        Set NULL
                      </label>
                    )}
                  </div>
                </div>

                {col.isGenerated ? (
                  <p className="text-[11px] italic text-slate-400">
                    Generated column (read-only)
                  </p>
                ) : d.useDefault ? (
                  <p className="text-[11px] font-mono text-emerald-600 dark:text-emerald-400">
                    Database default: {col.defaultValue || 'IDENTITY / AUTO'}
                  </p>
                ) : d.isNull ? (
                  <p className="text-[11px] font-mono italic text-slate-400">NULL</p>
                ) : isBool ? (
                  <select
                    value={d.value === 'true' ? 'true' : 'false'}
                    onChange={(e) => updateDraft(col.name, { value: e.target.value })}
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono"
                  >
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </select>
                ) : enumVals ? (
                  <select
                    value={d.value}
                    onChange={(e) => updateDraft(col.name, { value: e.target.value })}
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono"
                  >
                    {!enumVals.includes(d.value) && (
                      <option value={d.value}>{d.value || '-- select enum --'}</option>
                    )}
                    {enumVals.map((ev) => (
                      <option key={ev} value={ev}>
                        {ev}
                      </option>
                    ))}
                  </select>
                ) : isMultiline ? (
                  <textarea
                    rows={3}
                    value={d.value}
                    onChange={(e) => updateDraft(col.name, { value: e.target.value })}
                    placeholder={col.defaultValue ? `Default: ${col.defaultValue}` : ''}
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  />
                ) : (
                  <input
                    type="text"
                    value={d.value}
                    onChange={(e) => updateDraft(col.name, { value: e.target.value })}
                    placeholder={col.defaultValue ? `Default: ${col.defaultValue}` : ''}
                    className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  />
                )}
              </div>
            );
          })}
        </div>

        {error && (
          <div className="rounded-xl border border-red-500/40 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-600 dark:text-red-300">
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={saving}
            className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold"
          >
            {saving && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
            {mode === 'insert' ? 'Insert Row' : 'Save Changes'}
          </button>
        </div>
      </form>
    </Modal>
  );
}

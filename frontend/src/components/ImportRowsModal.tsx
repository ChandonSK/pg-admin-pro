import { useEffect, useState } from 'react';
import { FileSpreadsheet, Loader2, Upload } from 'lucide-react';
import { api } from '../api/client';
import type { ColumnInfo, ImportResult } from '../types';
import { Modal } from './Modal';

interface ImportRowsModalProps {
  open: boolean;
  dbName: string;
  schema: string;
  table: string;
  columns: ColumnInfo[];
  onClose: () => void;
  onImported: () => Promise<void>;
  onNotify: (kind: 'success' | 'error' | 'info', msg: string) => void;
}

function parseCSV(text: string): { headers: string[]; rows: string[][] } {
  const clean = text.replace(/^\uFEFF/, '');
  const records: string[][] = [];
  let row: string[] = [];
  let field = '';
  let inQuotes = false;

  for (let i = 0; i < clean.length; i++) {
    const ch = clean[i];
    if (inQuotes) {
      if (ch === '"' && clean[i + 1] === '"') {
        field += '"';
        i++;
      } else if (ch === '"') {
        inQuotes = false;
      } else {
        field += ch;
      }
      continue;
    }
    if (ch === '"') {
      inQuotes = true;
    } else if (ch === ',') {
      row.push(field);
      field = '';
    } else if (ch === '\r') {
      if (clean[i + 1] === '\n') i++;
      row.push(field);
      if (row.some((cell) => cell !== '')) records.push(row);
      row = [];
      field = '';
    } else if (ch === '\n') {
      row.push(field);
      if (row.some((cell) => cell !== '')) records.push(row);
      row = [];
      field = '';
    } else {
      field += ch;
    }
  }
  if (field !== '' || row.length > 0) {
    row.push(field);
    if (row.some((cell) => cell !== '')) records.push(row);
  }
  if (records.length === 0) return { headers: [], rows: [] };
  return {
    headers: records[0].map((h) => h.trim()),
    rows: records.slice(1),
  };
}

function coerceImportCell(raw: unknown, col: ColumnInfo | undefined, emptyAsNull: boolean): unknown {
  if (raw === null || raw === undefined) return null;
  if (typeof raw !== 'string') return raw;
  if (emptyAsNull && raw === '') return null;
  if (!col) return raw;
  const ft = col.formatType.toLowerCase();
  if (ft === 'boolean' || ft === 'bool') {
    if (raw.toLowerCase() === 'true' || raw === '1') return true;
    if (raw.toLowerCase() === 'false' || raw === '0') return false;
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

export function ImportRowsModal({
  open,
  dbName,
  schema,
  table,
  columns,
  onClose,
  onImported,
  onNotify,
}: ImportRowsModalProps) {
  const [sourceCols, setSourceCols] = useState<string[]>([]);
  const [rawRecords, setRawRecords] = useState<Record<string, unknown>[]>([]);
  const [colMap, setColMap] = useState<Record<string, string>>({});
  const [continueOnError, setContinueOnError] = useState(true);
  const [emptyAsNull, setEmptyAsNull] = useState(true);
  const [importing, setImporting] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [parseError, setParseError] = useState('');

  useEffect(() => {
    if (open) {
      setSourceCols([]);
      setRawRecords([]);
      setColMap({});
      setResult(null);
      setParseError('');
    }
  }, [open]);

  const loadContent = (filename: string, content: string) => {
    setParseError('');
    setResult(null);
    const trimmed = content.trim();
    if (!trimmed) {
      setParseError('File is empty');
      return;
    }

    if (filename.toLowerCase().endsWith('.json') || trimmed.startsWith('[')) {
      try {
        const arr = JSON.parse(trimmed) as unknown;
        if (!Array.isArray(arr) || arr.length === 0) {
          setParseError('JSON must be a non-empty array of row objects');
          return;
        }
        const recs = arr.filter(
          (x): x is Record<string, unknown> => Boolean(x && typeof x === 'object' && !Array.isArray(x))
        );
        const hdrSet = new Set<string>();
        recs.forEach((r) => Object.keys(r).forEach((k) => hdrSet.add(k)));
        const headers = Array.from(hdrSet);
        autoMapColumns(headers);
        setSourceCols(headers);
        setRawRecords(recs);
        return;
      } catch (err) {
        setParseError(err instanceof Error ? err.message : 'Invalid JSON');
        return;
      }
    }

    const parsed = parseCSV(content);
    if (parsed.headers.length === 0 || parsed.rows.length === 0) {
      setParseError('CSV must include a header row and at least one data row');
      return;
    }
    const recs = parsed.rows.map((vals) => {
      const obj: Record<string, unknown> = {};
      parsed.headers.forEach((h, i) => {
        obj[h] = vals[i] ?? '';
      });
      return obj;
    });
    autoMapColumns(parsed.headers);
    setSourceCols(parsed.headers);
    setRawRecords(recs);
  };

  const autoMapColumns = (headers: string[]) => {
    const map: Record<string, string> = {};
    for (const h of headers) {
      const exact = columns.find((c) => c.name === h && !c.isGenerated);
      const ci = exact || columns.find((c) => c.name.toLowerCase() === h.toLowerCase() && !c.isGenerated);
      map[h] = ci ? ci.name : '';
    }
    setColMap(map);
  };

  const handleFileChange = async (file: File | undefined) => {
    if (!file) return;
    const text = await file.text();
    loadContent(file.name, text);
  };

  const handleImport = async () => {
    const colByName = new Map(columns.map((c) => [c.name, c]));
    const activeMappings = Object.entries(colMap).filter(([, target]) => target !== '');
    if (activeMappings.length === 0) {
      onNotify('error', 'Map at least one column before importing');
      return;
    }

    const mappedRows = rawRecords.map((rec) => {
      const out: Record<string, unknown> = {};
      for (const [src, target] of activeMappings) {
        if (src in rec) {
          out[target] = coerceImportCell(rec[src], colByName.get(target), emptyAsNull);
        }
      }
      return out;
    });

    setImporting(true);
    try {
      const res = await api.tables.importRows(dbName, schema, table, {
        rows: mappedRows,
        continueOnError,
      });
      setResult(res);
      if (res.inserted > 0) {
        onNotify('success', `Imported ${res.inserted} row(s)${res.failed ? `, ${res.failed} failed` : ''}`);
        await onImported();
      } else if (res.failed > 0) {
        onNotify('error', `All ${res.failed} row(s) failed to import`);
      }
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Import failed');
    } finally {
      setImporting(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Import Data — ${schema}.${table}`}
      subtitle="Upload CSV or JSON array files and map columns safely"
      maxWidthClass="max-w-3xl"
    >
      <div className="space-y-4">
        <label className="flex flex-col items-center justify-center rounded-2xl border-2 border-dashed border-slate-300 dark:border-slate-700 hover:border-indigo-500 p-5 cursor-pointer bg-slate-50/70 dark:bg-slate-950/50 transition-colors">
          <Upload className="w-6 h-6 text-indigo-500 mb-1.5" />
          <span className="text-xs font-semibold text-slate-700 dark:text-slate-200">
            Choose .csv or .json file
          </span>
          <span className="text-[11px] text-slate-400 mt-0.5">
            First row of CSV must contain column headers; JSON must be an array of objects
          </span>
          <input
            type="file"
            accept=".csv,.json,text/csv,application/json"
            onChange={(e) => void handleFileChange(e.target.files?.[0])}
            className="sr-only"
          />
        </label>

        {parseError && (
          <div className="rounded-xl border border-red-500/40 bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-300">
            {parseError}
          </div>
        )}

        {sourceCols.length > 0 && (
          <>
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
              <div className="px-3.5 py-2 bg-slate-50 dark:bg-slate-950/60 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between text-xs font-semibold">
                <span className="flex items-center gap-1.5">
                  <FileSpreadsheet className="w-3.5 h-3.5 text-emerald-500" />
                  Column Mapping ({rawRecords.length.toLocaleString()} row(s) detected)
                </span>
              </div>
              <div className="max-h-56 overflow-y-auto divide-y divide-slate-100 dark:divide-slate-800">
                {sourceCols.map((src) => (
                  <div key={src} className="px-3.5 py-2 flex items-center justify-between gap-3 text-xs">
                    <span className="font-mono font-medium truncate">{src}</span>
                    <select
                      value={colMap[src] || ''}
                      onChange={(e) => setColMap((prev) => ({ ...prev, [src]: e.target.value }))}
                      className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-1 text-xs font-mono w-60"
                    >
                      <option value="">-- Skip column --</option>
                      {columns
                        .filter((c) => !c.isGenerated)
                        .map((c) => (
                          <option key={c.name} value={c.name}>
                            {c.name} ({c.formatType})
                          </option>
                        ))}
                    </select>
                  </div>
                ))}
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-4 text-xs">
              <label className="inline-flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={emptyAsNull}
                  onChange={(e) => setEmptyAsNull(e.target.checked)}
                  className="rounded border-slate-300 text-indigo-600"
                />
                Treat empty CSV cells as SQL NULL
              </label>
              <label className="inline-flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={continueOnError}
                  onChange={(e) => setContinueOnError(e.target.checked)}
                  className="rounded border-slate-300 text-indigo-600"
                />
                Continue valid rows if an error occurs (savepoints)
              </label>
            </div>
          </>
        )}

        {result && (
          <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950 p-3 space-y-2 text-xs">
            <div className="font-semibold">
              Inserted: <span className="text-emerald-500">{result.inserted}</span> &bull; Failed:{' '}
              <span className={result.failed > 0 ? 'text-red-500' : ''}>{result.failed}</span> &bull;{' '}
              {result.durationMs}ms
            </div>
            {result.errors && result.errors.length > 0 && (
              <div className="max-h-32 overflow-y-auto space-y-1 font-mono text-[11px] text-red-500">
                {result.errors.map((err, i) => (
                  <div key={i}>
                    Row #{err.rowIndex + 1}: {err.error}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        <div className="flex justify-end gap-2 pt-2">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
          >
            Close
          </button>
          <button
            type="button"
            disabled={importing || rawRecords.length === 0}
            onClick={() => void handleImport()}
            className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white text-xs font-semibold"
          >
            {importing && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
            Run Import ({rawRecords.length} rows)
          </button>
        </div>
      </div>
    </Modal>
  );
}

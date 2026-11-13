import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import {
  Bookmark,
  BookOpen,
  Download,
  GitBranch,
  Loader2,
  Play,
  Plus,
  Sparkles,
  Trash2,
  Wand2,
  X,
} from 'lucide-react';
import { api } from '../api/client';
import { useDismissible } from '../hooks/useDismissible';
import type {
  AutocompleteTable,
  ExplainResult,
  SQLResult,
  SQLSnippet,
} from '../types';
import { ConfirmModal } from './ConfirmModal';
import { ExplainPlanViewer } from './ExplainPlanViewer';
import { Modal } from './Modal';
import type { ToastItem } from './Toast';

interface SqlEditorTabProps {
  dbName: string;
  initialSql?: string;
  supportsExplain?: boolean;
  onNotify?: (kind: ToastItem['kind'], msg: string) => void;
}

interface SqlTab {
  id: string;
  title: string;
  sql: string;
}

interface SuggestionItem {
  label: string;
  insertText: string;
  kind: 'table' | 'column' | 'keyword';
  detail?: string;
}

const SQL_KEYWORDS = [
  'SELECT',
  'FROM',
  'WHERE',
  'AND',
  'OR',
  'NOT',
  'IN',
  'IS NULL',
  'IS NOT NULL',
  'ILIKE',
  'JOIN',
  'INNER JOIN',
  'LEFT JOIN',
  'ON',
  'GROUP BY',
  'HAVING',
  'ORDER BY',
  'ASC',
  'DESC',
  'LIMIT',
  'OFFSET',
  'INSERT INTO',
  'VALUES',
  'UPDATE',
  'SET',
  'DELETE FROM',
  'RETURNING',
  'CREATE TABLE',
  'ALTER TABLE',
  'DROP TABLE',
  'COUNT(*)',
  'SUM(',
  'AVG(',
  'DISTINCT',
  'COALESCE(',
  'CASE WHEN',
];

const BUILTIN_TEMPLATES = [
  {
    title: 'Active Queries (PostgreSQL)',
    tag: 'dba',
    sql: `SELECT pid, usename, state, now() - query_start AS duration, left(query, 120) AS query\nFROM pg_stat_activity\nWHERE datname = current_database()\n  AND state <> 'idle'\nORDER BY duration DESC;`,
  },
  {
    title: 'Largest Tables & Indexes',
    tag: 'dba',
    sql: `SELECT nspname || '.' || relname AS table_name,\n       pg_size_pretty(pg_total_relation_size(c.oid)) AS total_size,\n       c.reltuples::bigint AS row_estimate\nFROM pg_class c\nJOIN pg_namespace n ON n.oid = c.relnamespace\nWHERE c.relkind IN ('r', 'p') AND n.nspname NOT IN ('pg_catalog', 'information_schema')\nORDER BY pg_total_relation_size(c.oid) DESC\nLIMIT 25;`,
  },
  {
    title: 'Index Usage vs Seq Scans',
    tag: 'dba',
    sql: `SELECT schemaname, relname, seq_scan, idx_scan, n_live_tup, n_dead_tup\nFROM pg_stat_user_tables\nORDER BY seq_scan DESC\nLIMIT 25;`,
  },
];

function formatSqlBasic(input: string): string {
  const placeholders: string[] = [];
  const masked = input.replace(/'([^']|'')*'|"([^"]|"")*"/g, (m) => {
    const idx = placeholders.push(m) - 1;
    return `__SQLSTR_${idx}__`;
  });

  const majorBreaks = new Set([
    'SELECT',
    'FROM',
    'WHERE',
    'GROUP BY',
    'HAVING',
    'ORDER BY',
    'LIMIT',
    'OFFSET',
    'VALUES',
    'SET',
    'RETURNING',
    'INNER JOIN',
    'LEFT JOIN',
    'RIGHT JOIN',
    'FULL JOIN',
    'JOIN',
    'UNION',
    'UNION ALL',
  ]);

  let normalized = masked.replace(/\s+/g, ' ').trim();
  const multiKeywords = [
    'GROUP BY',
    'ORDER BY',
    'INNER JOIN',
    'LEFT JOIN',
    'RIGHT JOIN',
    'FULL JOIN',
    'UNION ALL',
    'IS NOT NULL',
    'IS NULL',
    'INSERT INTO',
    'DELETE FROM',
  ];
  for (const mk of multiKeywords) {
    normalized = normalized.replace(new RegExp(`\\b${mk}\\b`, 'gi'), mk);
  }
  const singleKeywords = [
    'SELECT',
    'FROM',
    'WHERE',
    'AND',
    'OR',
    'HAVING',
    'LIMIT',
    'OFFSET',
    'JOIN',
    'ON',
    'AS',
    'IN',
    'ASC',
    'DESC',
    'VALUES',
    'UPDATE',
    'SET',
    'RETURNING',
    'DISTINCT',
    'CASE',
    'WHEN',
    'THEN',
    'ELSE',
    'END',
  ];
  for (const sk of singleKeywords) {
    normalized = normalized.replace(new RegExp(`\\b${sk}\\b`, 'gi'), sk);
  }

  for (const brk of majorBreaks) {
    if (brk === 'JOIN' || brk.endsWith(' JOIN') || brk === 'FROM' || brk === 'WHERE' || brk === 'GROUP BY' || brk === 'HAVING' || brk === 'ORDER BY' || brk === 'LIMIT' || brk === 'OFFSET' || brk === 'VALUES' || brk === 'SET' || brk === 'RETURNING') {
      normalized = normalized.replace(new RegExp(`\\s+(${brk})\\b`, 'g'), `\n$1`);
    }
  }
  normalized = normalized.replace(/\s+(AND|OR)\b/g, '\n  $1');
  normalized = normalized.replace(/,\s*/g, ',\n  ');

  return normalized.replace(/__SQLSTR_(\d+)__/g, (_, i) => placeholders[Number(i)]);
}

function loadInitialTabs(dbName: string, initialSql?: string): SqlTab[] {
  const key = `pg_admin_pro_sql_tabs_${dbName}`;
  try {
    const raw = localStorage.getItem(key);
    if (raw) {
      const parsed = JSON.parse(raw) as SqlTab[];
      if (Array.isArray(parsed) && parsed.length > 0) {
        return parsed;
      }
    }
  } catch {
    // ignore storage errors
  }
  return [
    {
      id: 'tab-1',
      title: 'Query 1',
      sql: initialSql || 'SELECT now() AS server_time, current_database(), current_user;',
    },
  ];
}

export function SqlEditorTab({
  dbName,
  initialSql,
  supportsExplain = true,
  onNotify,
}: SqlEditorTabProps) {
  const [tabs, setTabs] = useState<SqlTab[]>(() => loadInitialTabs(dbName, initialSql));
  const [activeTabId, setActiveTabId] = useState<string>(() => tabs[0]?.id || 'tab-1');
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState('');

  const [running, setRunning] = useState(false);
  const [explaining, setExplaining] = useState(false);
  const [result, setResult] = useState<SQLResult | null>(null);
  const [explainResult, setExplainResult] = useState<ExplainResult | null>(null);
  const [error, setError] = useState('');
  const [history, setHistory] = useState<string[]>([]);

  // Autocomplete & Snippets
  const [catalog, setCatalog] = useState<AutocompleteTable[]>([]);
  const [acOpen, setAcOpen] = useState(false);
  const [acIndex, setAcIndex] = useState(0);
  const [acPrefix, setAcPrefix] = useState('');
  const [acTableScope, setAcTableScope] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const editorBoxRef = useRef<HTMLDivElement>(null);
  const closeAc = useCallback(() => setAcOpen(false), []);
  useDismissible(editorBoxRef, acOpen, closeAc);

  const [snippetsOpen, setSnippetsOpen] = useState(false);
  const [snippets, setSnippets] = useState<SQLSnippet[]>([]);
  const [saveOpen, setSaveOpen] = useState(false);
  const [snipTitle, setSnipTitle] = useState('');
  const [snipTag, setSnipTag] = useState('queries');
  const [savingSnip, setSavingSnip] = useState(false);
  const [deleteSnip, setDeleteSnip] = useState<SQLSnippet | null>(null);

  useEffect(() => {
    const nextTabs = loadInitialTabs(dbName, initialSql);
    setTabs(nextTabs);
    setActiveTabId(nextTabs[0]?.id || 'tab-1');
    setResult(null);
    setExplainResult(null);
    setError('');
    api.tables
      .autocomplete(dbName)
      .then((res) => setCatalog(res.tables || []))
      .catch(() => setCatalog([]));
  }, [dbName]);

  useEffect(() => {
    try {
      localStorage.setItem(`pg_admin_pro_sql_tabs_${dbName}`, JSON.stringify(tabs));
    } catch {
      // ignore quota
    }
  }, [dbName, tabs]);

  useEffect(() => {
    if (snippetsOpen) {
      api.snippets
        .list()
        .then((res) => setSnippets(res.snippets || []))
        .catch(() => {});
    }
  }, [snippetsOpen]);

  const activeTab = tabs.find((t) => t.id === activeTabId) || tabs[0];
  const sql = activeTab?.sql ?? '';

  const updateActiveSql = (nextSql: string) => {
    setTabs((prev) =>
      prev.map((t) => (t.id === activeTab.id ? { ...t, sql: nextSql } : t))
    );
  };

  const addTab = (seedSql?: string, title?: string) => {
    const id = `tab-${Date.now()}`;
    const num = tabs.length + 1;
    const next: SqlTab = {
      id,
      title: title || `Query ${num}`,
      sql: seedSql ?? initialSql ?? '',
    };
    setTabs((prev) => [...prev, next]);
    setActiveTabId(id);
  };

  const closeTab = (id: string) => {
    if (tabs.length <= 1) return;
    const remaining = tabs.filter((t) => t.id !== id);
    setTabs(remaining);
    if (activeTabId === id) {
      setActiveTabId(remaining[remaining.length - 1].id);
    }
  };

  const suggestions: SuggestionItem[] = useMemo(() => {
    if (!acOpen) return [];
    const low = acPrefix.toLowerCase();
    const out: SuggestionItem[] = [];

    if (acTableScope) {
      const tbl = catalog.find(
        (t) => t.name.toLowerCase() === acTableScope.toLowerCase()
      );
      if (tbl) {
        for (const col of tbl.columns) {
          if (!low || col.toLowerCase().includes(low)) {
            out.push({
              label: col,
              insertText: col,
              kind: 'column',
              detail: `${tbl.schema}.${tbl.name}`,
            });
          }
        }
        return out.slice(0, 12);
      }
    }

    if (!low) return [];

    for (const tbl of catalog) {
      if (tbl.name.toLowerCase().startsWith(low)) {
        out.push({
          label: `${tbl.schema}.${tbl.name}`,
          insertText: tbl.name,
          kind: 'table',
          detail: tbl.type,
        });
      }
    }
    for (const kw of SQL_KEYWORDS) {
      if (kw.toLowerCase().startsWith(low)) {
        out.push({ label: kw, insertText: kw, kind: 'keyword' });
      }
    }
    for (const tbl of catalog) {
      for (const col of tbl.columns) {
        if (col.toLowerCase().startsWith(low) && !out.some((x) => x.label === col)) {
          out.push({
            label: col,
            insertText: col,
            kind: 'column',
            detail: tbl.name,
          });
        }
      }
    }
    return out.slice(0, 12);
  }, [acOpen, acPrefix, acTableScope, catalog]);

  const inspectAutocompleteAtCursor = (text: string, cursorPos: number, force = false) => {
    const before = text.slice(0, cursorPos);
    const dotMatch = before.match(/([a-zA-Z_][\w]*)\.([a-zA-Z_0-9]*)$/);
    if (dotMatch) {
      setAcTableScope(dotMatch[1]);
      setAcPrefix(dotMatch[2]);
      setAcOpen(true);
      setAcIndex(0);
      return;
    }
    const wordMatch = before.match(/([a-zA-Z_][\w]{1,})$/);
    if (wordMatch) {
      setAcTableScope('');
      setAcPrefix(wordMatch[1]);
      setAcOpen(true);
      setAcIndex(0);
      return;
    }
    if (force) {
      setAcTableScope('');
      setAcPrefix('');
      setAcOpen(true);
    } else {
      setAcOpen(false);
    }
  };

  const applySuggestion = (item: SuggestionItem) => {
    const el = textareaRef.current;
    if (!el) return;
    const pos = el.selectionStart ?? sql.length;
    const before = sql.slice(0, pos);
    const after = sql.slice(pos);
    const replaceLen = acPrefix.length;
    const nextSql = before.slice(0, before.length - replaceLen) + item.insertText + after;
    const nextPos = pos - replaceLen + item.insertText.length;
    updateActiveSql(nextSql);
    setAcOpen(false);
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(nextPos, nextPos);
    });
  };

  const execute = async () => {
    if (!sql.trim()) return;
    setRunning(true);
    setError('');
    setAcOpen(false);
    try {
      const res = await api.sql.execute(dbName, sql);
      setResult(res);
      setExplainResult(null);
      setHistory((prev) => [sql.trim(), ...prev.filter((q) => q !== sql.trim())].slice(0, 10));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Query failed');
      setResult(null);
    } finally {
      setRunning(false);
    }
  };

  const runExplain = async (analyze: boolean) => {
    if (!sql.trim()) return;
    setExplaining(true);
    setError('');
    setAcOpen(false);
    try {
      const res = await api.sql.explain(dbName, sql, analyze);
      setExplainResult(res);
      setResult(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Explain failed');
    } finally {
      setExplaining(false);
    }
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      void execute();
      return;
    }
    if ((e.ctrlKey || e.metaKey) && e.code === 'Space') {
      e.preventDefault();
      inspectAutocompleteAtCursor(sql, e.currentTarget.selectionStart ?? sql.length, true);
      return;
    }
    if (acOpen && suggestions.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setAcIndex((i) => (i + 1) % suggestions.length);
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setAcIndex((i) => (i - 1 + suggestions.length) % suggestions.length);
        return;
      }
      if (e.key === 'Tab' || e.key === 'Enter') {
        e.preventDefault();
        applySuggestion(suggestions[acIndex] || suggestions[0]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setAcOpen(false);
      }
    }
  };

  const exportCSV = () => {
    if (!result || !result.columns.length) return;
    const esc = (v: unknown) => {
      if (v === null || v === undefined) return '';
      const s = typeof v === 'object' ? JSON.stringify(v) : String(v);
      return `"${s.replace(/"/g, '""')}"`;
    };
    const lines = [
      result.columns.map(esc).join(','),
      ...result.rows.map((r) => r.map(esc).join(',')),
    ];
    const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${dbName}_query.csv`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleSaveSnippet = async () => {
    if (!snipTitle.trim() || !sql.trim()) return;
    setSavingSnip(true);
    try {
      await api.snippets.save({
        title: snipTitle.trim(),
        tag: snipTag.trim() || 'queries',
        sql: sql.trim(),
      });
      onNotify?.('success', `Saved snippet "${snipTitle.trim()}"`);
      setSaveOpen(false);
      setSnipTitle('');
      if (snippetsOpen) {
        const res = await api.snippets.list();
        setSnippets(res.snippets || []);
      }
    } catch (err) {
      onNotify?.('error', err instanceof Error ? err.message : 'Save snippet failed');
    } finally {
      setSavingSnip(false);
    }
  };

  const handleDeleteSnippet = async () => {
    if (!deleteSnip) return;
    try {
      await api.snippets.remove(deleteSnip.id);
      setSnippets((prev) => prev.filter((s) => s.id !== deleteSnip.id));
      setDeleteSnip(null);
      onNotify?.('success', 'Snippet deleted');
    } catch (err) {
      onNotify?.('error', err instanceof Error ? err.message : 'Delete snippet failed');
    }
  };

  return (
    <div className="space-y-4">
      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/80 overflow-hidden">
        {/* Multi-tab strip */}
        <div className="px-3 pt-2 bg-slate-100 dark:bg-slate-950 border-b border-slate-200 dark:border-slate-800 flex items-center gap-1 overflow-x-auto">
          {tabs.map((t) => {
            const active = t.id === activeTab.id;
            return (
              <div
                key={t.id}
                className={`group flex items-center gap-1.5 px-3 py-1.5 rounded-t-xl text-xs font-medium border-t border-x cursor-pointer shrink-0 transition-colors ${
                  active
                    ? 'bg-slate-950 text-white border-slate-800'
                    : 'bg-white/60 dark:bg-slate-900/60 border-transparent text-slate-600 dark:text-slate-400 hover:bg-white dark:hover:bg-slate-900'
                }`}
                onClick={() => setActiveTabId(t.id)}
                onDoubleClick={() => {
                  setRenamingId(t.id);
                  setRenameDraft(t.title);
                }}
              >
                {renamingId === t.id ? (
                  <input
                    type="text"
                    autoFocus
                    value={renameDraft}
                    onChange={(e) => setRenameDraft(e.target.value)}
                    onBlur={() => {
                      if (renameDraft.trim()) {
                        setTabs((prev) =>
                          prev.map((x) =>
                            x.id === t.id ? { ...x, title: renameDraft.trim() } : x
                          )
                        );
                      }
                      setRenamingId(null);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === 'Escape') {
                        (e.target as HTMLInputElement).blur();
                      }
                    }}
                    className="w-24 bg-slate-800 text-white px-1 rounded text-xs"
                  />
                ) : (
                  <span>{t.title}</span>
                )}
                {tabs.length > 1 && (
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      closeTab(t.id);
                    }}
                    aria-label={`Close ${t.title}`}
                    className="opacity-60 hover:opacity-100"
                  >
                    <X className="w-3 h-3" />
                  </button>
                )}
              </div>
            );
          })}
          <button
            type="button"
            onClick={() => addTab()}
            title="New SQL tab"
            className="p-1.5 rounded-lg text-slate-500 hover:text-indigo-500 hover:bg-slate-200 dark:hover:bg-slate-800 mb-1"
          >
            <Plus className="w-3.5 h-3.5" />
          </button>
        </div>

        {/* Toolbar */}
        <div className="px-4 py-2 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2 text-xs font-semibold">
            <Sparkles className="w-3.5 h-3.5 text-indigo-500" />
            <span>SQL IDE</span>
            <span className="text-[11px] font-normal text-slate-400">
              {catalog.length > 0 ? `• ${catalog.length} tables indexed for autocomplete` : ''}
            </span>
          </div>

          <div className="flex flex-wrap items-center gap-1.5">
            <button
              type="button"
              onClick={() => updateActiveSql(formatSqlBasic(sql))}
              title="Format SQL keywords and clauses"
              className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs"
            >
              <Wand2 className="w-3.5 h-3.5 text-amber-500" />
              Format
            </button>

            <button
              type="button"
              onClick={() => setSaveOpen(true)}
              title="Save active query as reusable snippet"
              className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs"
            >
              <Bookmark className="w-3.5 h-3.5 text-emerald-500" />
              Save
            </button>

            <button
              type="button"
              onClick={() => setSnippetsOpen((v) => !v)}
              className={`inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border text-xs ${
                snippetsOpen
                  ? 'border-indigo-500 bg-indigo-500/10 text-indigo-500'
                  : 'border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800'
              }`}
            >
              <BookOpen className="w-3.5 h-3.5" />
              Snippets
            </button>

            {supportsExplain && (
              <>
                <button
                  type="button"
                  disabled={explaining || running}
                  onClick={() => void runExplain(false)}
                  title="Show estimated planner execution tree"
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium"
                >
                  <GitBranch className="w-3.5 h-3.5 text-cyan-500" />
                  Explain
                </button>
                <button
                  type="button"
                  disabled={explaining || running}
                  onClick={() => void runExplain(true)}
                  title="Executes query in a rolled-back transaction to measure actual timings safely"
                  className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-cyan-500/30 text-cyan-600 dark:text-cyan-300 hover:bg-cyan-500/10 text-xs font-medium"
                >
                  {explaining ? (
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    <GitBranch className="w-3.5 h-3.5" />
                  )}
                  Analyze
                </button>
              </>
            )}

            <button
              type="button"
              disabled={running || explaining}
              onClick={() => void execute()}
              className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold"
            >
              {running ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
              ) : (
                <Play className="w-3.5 h-3.5" />
              )}
              Run (Ctrl+Enter)
            </button>
          </div>
        </div>

        {/* Optional Snippets Drawer */}
        {snippetsOpen && (
          <div className="p-3 bg-slate-50 dark:bg-slate-950/90 border-b border-slate-200 dark:border-slate-800 space-y-2.5">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-2">
              {snippets.map((sn) => (
                <div
                  key={sn.id}
                  className="flex items-center justify-between gap-2 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 p-2.5 text-xs"
                >
                  <button
                    type="button"
                    onClick={() => {
                      updateActiveSql(sn.sql);
                      setSnippetsOpen(false);
                    }}
                    className="text-left min-w-0 flex-1 hover:text-indigo-500"
                  >
                    <div className="font-semibold truncate">{sn.title}</div>
                    <div className="text-[10px] font-mono text-slate-400 truncate">
                      [{sn.tag}] {sn.sql}
                    </div>
                  </button>
                  <button
                    type="button"
                    onClick={() => setDeleteSnip(sn)}
                    aria-label={`Delete snippet ${sn.title}`}
                    className="p-1 text-slate-400 hover:text-red-500 shrink-0"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))}
              {BUILTIN_TEMPLATES.map((tpl) => (
                <button
                  key={tpl.title}
                  type="button"
                  onClick={() => {
                    updateActiveSql(tpl.sql);
                    setSnippetsOpen(false);
                  }}
                  className="text-left rounded-xl border border-dashed border-slate-300 dark:border-slate-700 bg-white/50 dark:bg-slate-900/50 hover:border-indigo-500 p-2.5 text-xs"
                >
                  <div className="font-semibold text-indigo-500 truncate">{tpl.title}</div>
                  <div className="text-[10px] font-mono text-slate-400 truncate">
                    [built-in template]
                  </div>
                </button>
              ))}
            </div>
          </div>
        )}

        {/* SQL Editor + Schema Autocomplete Popup */}
        <div ref={editorBoxRef} className="relative">
          <textarea
            ref={textareaRef}
            rows={8}
            value={sql}
            onChange={(e) => {
              const val = e.target.value;
              updateActiveSql(val);
              inspectAutocompleteAtCursor(val, e.target.selectionStart ?? val.length);
            }}
            onKeyDown={handleKeyDown}
            spellCheck={false}
            className="w-full bg-slate-950 text-slate-100 p-4 text-xs font-mono leading-relaxed focus:outline-none resize-y"
            placeholder="Write SQL query... (type table. for column suggestions or Ctrl+Space)"
          />

          {acOpen && suggestions.length > 0 && (
            <div className="absolute left-4 bottom-3 z-30 w-72 rounded-xl border border-slate-700 bg-slate-900 shadow-2xl overflow-hidden text-xs">
              <div className="px-2.5 py-1 bg-slate-950 text-[10px] text-slate-400 border-b border-slate-800 flex justify-between">
                <span>Suggestions ({acTableScope ? `${acTableScope}.*` : 'schema'})</span>
                <span>Tab / Enter</span>
              </div>
              <div className="max-h-48 overflow-y-auto">
                {suggestions.map((item, i) => (
                  <button
                    key={`${item.kind}-${item.label}`}
                    type="button"
                    onMouseDown={(e) => {
                      e.preventDefault();
                      applySuggestion(item);
                    }}
                    className={`w-full px-3 py-1.5 flex items-center justify-between gap-2 text-left font-mono ${
                      i === acIndex
                        ? 'bg-indigo-600 text-white'
                        : 'text-slate-200 hover:bg-slate-800'
                    }`}
                  >
                    <span className="truncate">{item.label}</span>
                    <span
                      className={`text-[10px] uppercase shrink-0 ${
                        i === acIndex ? 'text-indigo-100' : 'text-slate-400'
                      }`}
                    >
                      {item.detail || item.kind}
                    </span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>

        {history.length > 0 && (
          <div className="px-4 py-2 border-t border-slate-200 dark:border-slate-800 flex items-center gap-2 overflow-x-auto text-[11px]">
            <span className="text-slate-400 shrink-0">History:</span>
            {history.map((q, i) => (
              <button
                key={i}
                type="button"
                onClick={() => updateActiveSql(q)}
                className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 hover:bg-indigo-500/20 font-mono truncate max-w-[220px] shrink-0"
              >
                {q}
              </button>
            ))}
          </div>
        )}
      </div>

      {error && (
        <div
          role="alert"
          className="rounded-xl border border-red-500/40 bg-red-500/10 p-3.5 text-xs font-mono text-red-600 dark:text-red-300"
        >
          {error}
        </div>
      )}

      {explainResult && <ExplainPlanViewer result={explainResult} />}

      {result && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
          <div className="px-4 py-2.5 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between text-xs">
            <div className="flex items-center gap-3">
              <span className="font-mono font-semibold text-emerald-500">
                {result.commandTag || 'OK'}
              </span>
              <span className="text-slate-400 font-mono">{result.durationMs}ms</span>
              {result.isSelect && (
                <span className="text-slate-400">{result.rows.length} rows returned</span>
              )}
            </div>

            {result.isSelect && result.rows.length > 0 && (
              <button
                type="button"
                onClick={exportCSV}
                className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-[11px]"
              >
                <Download className="w-3 h-3" /> Export CSV
              </button>
            )}
          </div>

          {result.isSelect && (
            <div className="max-h-[420px] overflow-auto">
              <table className="w-full text-left text-xs font-mono">
                <thead className="sticky top-0 bg-slate-100 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">
                  <tr>
                    {result.columns.map((c, i) => (
                      <th key={i} className="px-3 py-2 font-semibold whitespace-nowrap">
                        {c}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70">
                  {result.rows.map((row, rIdx) => (
                    <tr key={rIdx} className="hover:bg-slate-50 dark:hover:bg-slate-800/40">
                      {row.map((val, cIdx) => (
                        <td key={cIdx} className="px-3 py-2 max-w-[280px] truncate whitespace-nowrap">
                          {val === null ? (
                            <span className="text-slate-400 italic">NULL</span>
                          ) : typeof val === 'object' ? (
                            JSON.stringify(val)
                          ) : (
                            String(val)
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* Save Snippet Modal */}
      <Modal
        open={saveOpen}
        onClose={() => setSaveOpen(false)}
        title="Save SQL Snippet"
        subtitle="Reusable across sessions and databases"
      >
        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium mb-1">Snippet Title</label>
            <input
              type="text"
              autoFocus
              value={snipTitle}
              onChange={(e) => setSnipTitle(e.target.value)}
              placeholder="e.g. Monthly Fee Summary"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs"
            />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Tag</label>
            <input
              type="text"
              value={snipTag}
              onChange={(e) => setSnipTag(e.target.value)}
              placeholder="queries"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>
          <div className="flex justify-end gap-2 pt-1">
            <button
              type="button"
              onClick={() => setSaveOpen(false)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="button"
              disabled={savingSnip || !snipTitle.trim() || !sql.trim()}
              onClick={() => void handleSaveSnippet()}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 text-white text-xs font-semibold"
            >
              {savingSnip && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Save Snippet
            </button>
          </div>
        </div>
      </Modal>

      <ConfirmModal
        open={!!deleteSnip}
        onClose={() => setDeleteSnip(null)}
        title="Delete Snippet"
        message={
          <>
            Delete saved snippet <strong>{deleteSnip?.title}</strong>?
          </>
        }
        confirmLabel="Delete Snippet"
        onConfirm={handleDeleteSnippet}
      />
    </div>
  );
}

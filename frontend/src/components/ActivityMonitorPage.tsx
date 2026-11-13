import { useCallback, useEffect, useState } from 'react';
import {
  Activity,
  AlertOctagon,
  Database,
  Flame,
  HardDrive,
  Lock,
  Play,
  RefreshCw,
  Skull,
  Sparkles,
  Trash2,
  XOctagon,
} from 'lucide-react';
import { api } from '../api/client';
import type {
  ActivityOverview,
  ActivitySession,
  DatabaseInfo,
  ServerOverview,
  UnusedIndexInfo,
} from '../types';
import { ConfirmModal } from './ConfirmModal';
import { Modal } from './Modal';
import type { ToastItem } from './Toast';

interface ActivityMonitorPageProps {
  overview: ServerOverview;
  databases: DatabaseInfo[];
  onNotify: (kind: ToastItem['kind'], message: string) => void;
}

type MonitorTab = 'sessions' | 'locks' | 'tables' | 'indexes';

export function ActivityMonitorPage({
  overview,
  databases,
  onNotify,
}: ActivityMonitorPageProps) {
  const [selectedDB, setSelectedDB] = useState(overview.currentDatabase || '');
  const [data, setData] = useState<ActivityOverview | null>(null);
  const [loading, setLoading] = useState(false);
  const [tab, setTab] = useState<MonitorTab>('sessions');
  const [pollMs, setPollMs] = useState<number>(5000);
  const [showIdle, setShowIdle] = useState(false);
  const [inspectedQuery, setInspectedQuery] = useState<ActivitySession | null>(null);
  const [pidConfirm, setPidConfirm] = useState<{
    session: ActivitySession;
    terminate: boolean;
  } | null>(null);
  const [dropIdxConfirm, setDropIdxConfirm] = useState<UnusedIndexInfo | null>(null);
  const [acting, setActing] = useState(false);

  const loadActivity = useCallback(
    async (silent = false) => {
      if (!silent) setLoading(true);
      try {
        const res = await api.monitor.activity(selectedDB);
        setData(res);
      } catch (err) {
        if (!silent) {
          onNotify('error', err instanceof Error ? err.message : 'Failed to load activity monitor');
        }
      } finally {
        if (!silent) setLoading(false);
      }
    },
    [selectedDB, onNotify]
  );

  useEffect(() => {
    void loadActivity(false);
  }, [loadActivity]);

  useEffect(() => {
    if (pollMs <= 0) return;
    const id = window.setInterval(() => {
      void loadActivity(true);
    }, pollMs);
    return () => window.clearInterval(id);
  }, [pollMs, loadActivity]);

  const handlePidAction = async () => {
    if (!pidConfirm) return;
    setActing(true);
    try {
      if (pidConfirm.terminate) {
        await api.monitor.terminate(pidConfirm.session.pid);
        onNotify('success', `Terminated backend PID ${pidConfirm.session.pid}`);
      } else {
        await api.monitor.cancel(pidConfirm.session.pid);
        onNotify('success', `Cancelled active query on PID ${pidConfirm.session.pid}`);
      }
      setPidConfirm(null);
      await loadActivity(true);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Backend signal failed');
    } finally {
      setActing(false);
    }
  };

  const handleMaintain = async (
    schema: string,
    table: string,
    action: 'vacuum_analyze' | 'analyze' | 'reindex'
  ) => {
    const dbTarget = selectedDB || overview.currentDatabase;
    try {
      const res = await api.tables.maintain(dbTarget, schema, table, action);
      onNotify('success', `${action.toUpperCase()} finished on ${schema}.${table} (${res.durationMs}ms)`);
      await loadActivity(true);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Maintenance command failed');
    }
  };

  const handleDropUnusedIndex = async () => {
    if (!dropIdxConfirm) return;
    const dbTarget = selectedDB || overview.currentDatabase;
    setActing(true);
    try {
      await api.tables.dropIndex(
        dbTarget,
        dropIdxConfirm.schema,
        dropIdxConfirm.table,
        dropIdxConfirm.indexName
      );
      onNotify('success', `Dropped unused index "${dropIdxConfirm.indexName}"`);
      setDropIdxConfirm(null);
      await loadActivity(true);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to drop index');
    } finally {
      setActing(false);
    }
  };

  const visibleSessions = (data?.sessions || []).filter((s) =>
    showIdle ? true : s.state !== 'idle' || (s.blockingPids && s.blockingPids.length > 0)
  );

  const waitingLocks = (data?.locks || []).filter((l) => !l.granted).length;

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold tracking-tight flex items-center gap-2">
            <Activity className="w-5 h-5 text-indigo-500" />
            Activity, Locks &amp; Performance Monitor
          </h1>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
            Live backend queries, lock contention, table bloat, and unused index insights
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1.5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 px-2.5 py-1.5">
            <Database className="w-3.5 h-3.5 text-slate-400" />
            <label htmlFor="mon-db-select" className="text-[11px] text-slate-500">
              DB Scope:
            </label>
            <select
              id="mon-db-select"
              value={selectedDB}
              onChange={(e) => setSelectedDB(e.target.value)}
              className="bg-transparent text-xs font-mono font-medium focus:outline-none"
            >
              <option value="">All Databases (Cluster)</option>
              {databases
                .filter((d) => d.allowConnect)
                .map((d) => (
                  <option key={d.name} value={d.name}>
                    {d.name}
                  </option>
                ))}
            </select>
          </div>

          <select
            aria-label="Auto refresh interval"
            value={pollMs}
            onChange={(e) => setPollMs(Number(e.target.value))}
            className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-medium"
          >
            <option value={0}>Auto: Off</option>
            <option value={2000}>Auto: 2s</option>
            <option value={5000}>Auto: 5s</option>
            <option value={15000}>Auto: 15s</option>
          </select>

          <button
            type="button"
            onClick={() => void loadActivity(false)}
            disabled={loading}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-xs font-medium hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
        </div>
      </div>

      {/* KPI Metrics Cards */}
      {data && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3.5">
          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 p-4">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
              <span>Buffer Cache Hit Ratio</span>
              <Flame className="w-4 h-4 text-emerald-500" />
            </div>
            <div className="mt-2 flex items-baseline gap-2">
              <span
                className={`text-2xl font-bold font-mono ${
                  data.metrics.cacheHitRatio >= 98
                    ? 'text-emerald-500'
                    : data.metrics.cacheHitRatio >= 90
                    ? 'text-amber-500'
                    : 'text-red-500'
                }`}
              >
                {data.metrics.cacheHitRatio.toFixed(2)}%
              </span>
              <span className="text-[11px] text-slate-400">Target &ge; 99%</span>
            </div>
          </div>

          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 p-4">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
              <span>Connections Pool</span>
              <Activity className="w-4 h-4 text-indigo-500" />
            </div>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-2xl font-bold font-mono">{data.metrics.activeCount}</span>
              <span className="text-xs text-slate-500">
                active &middot; {data.metrics.idleCount} idle &middot; max {data.metrics.maxConnections}
              </span>
            </div>
            {data.metrics.waitingCount > 0 && (
              <p className="text-[11px] text-amber-500 font-medium mt-1">
                {data.metrics.waitingCount} waiting on locks
              </p>
            )}
          </div>

          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 p-4">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
              <span>Transactions &amp; Deadlocks</span>
              <AlertOctagon
                className={`w-4 h-4 ${
                  data.metrics.deadlocks > 0 ? 'text-red-500' : 'text-slate-400'
                }`}
              />
            </div>
            <div className="mt-2 flex items-baseline gap-2 font-mono">
              <span className="text-lg font-bold text-slate-900 dark:text-white">
                {data.metrics.commits.toLocaleString()}
              </span>
              <span className="text-[11px] text-slate-400">
                commits / {data.metrics.rollbacks.toLocaleString()} rollbacks
              </span>
            </div>
            <p className="text-[11px] text-slate-500 mt-0.5">
              Deadlocks:{' '}
              <span className={data.metrics.deadlocks > 0 ? 'text-red-500 font-semibold' : ''}>
                {data.metrics.deadlocks}
              </span>
            </p>
          </div>

          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 p-4">
            <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
              <span>Temp Disk Spill</span>
              <HardDrive className="w-4 h-4 text-sky-500" />
            </div>
            <div className="mt-2 text-2xl font-bold font-mono">
              {data.metrics.tempBytesPretty || '0 B'}
            </div>
            <p className="text-[11px] text-slate-400 mt-0.5">
              Cumulative sort/hash temp I/O
            </p>
          </div>
        </div>
      )}

      {/* Sub-navigation */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 dark:border-slate-800 pb-2">
        <div className="flex flex-wrap items-center gap-1.5">
          {[
            {
              id: 'sessions' as const,
              label: `Live Queries (${data?.sessions.length ?? 0})`,
              icon: Play,
            },
            {
              id: 'locks' as const,
              label: `Locks (${data?.locks.length ?? 0})${waitingLocks > 0 ? ` • ${waitingLocks} blocked` : ''}`,
              icon: Lock,
            },
            {
              id: 'tables' as const,
              label: `Table Disk & Bloat (${data?.tableStats.length ?? 0})`,
              icon: HardDrive,
            },
            {
              id: 'indexes' as const,
              label: `Unused Indexes (${data?.unusedIndexes.length ?? 0})`,
              icon: Sparkles,
            },
          ].map((item) => {
            const Icon = item.icon;
            const active = tab === item.id;
            return (
              <button
                key={item.id}
                type="button"
                onClick={() => setTab(item.id)}
                className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-semibold transition-colors ${
                  active
                    ? 'bg-indigo-600 text-white'
                    : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <Icon className="w-3.5 h-3.5" />
                {item.label}
              </button>
            );
          })}
        </div>

        {tab === 'sessions' && (
          <label className="inline-flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300 cursor-pointer">
            <input
              type="checkbox"
              checked={showIdle}
              onChange={(e) => setShowIdle(e.target.checked)}
              className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
            />
            Show idle sessions
          </label>
        )}
      </div>

      {/* Tab Content */}
      {tab === 'sessions' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-950/60 border-b border-slate-200 dark:border-slate-800 text-[11px] uppercase tracking-wider text-slate-500">
                <tr>
                  <th className="px-3 py-2.5">PID</th>
                  <th className="px-3 py-2.5">Database</th>
                  <th className="px-3 py-2.5">User / App</th>
                  <th className="px-3 py-2.5">State / Wait</th>
                  <th className="px-3 py-2.5">Duration</th>
                  <th className="px-3 py-2.5">SQL Query</th>
                  <th className="px-3 py-2.5 text-right">Control</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
                {visibleSessions.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="px-4 py-8 text-center text-slate-400">
                      No matching active sessions. Toggle "Show idle sessions" to inspect idle pool clients.
                    </td>
                  </tr>
                ) : (
                  visibleSessions.map((s) => (
                    <tr
                      key={s.pid}
                      className="hover:bg-slate-50/70 dark:hover:bg-slate-800/40 transition-colors"
                    >
                      <td className="px-3 py-2.5 font-mono font-semibold text-indigo-600 dark:text-indigo-400">
                        {s.pid}
                      </td>
                      <td className="px-3 py-2.5 font-mono">{s.database || '—'}</td>
                      <td className="px-3 py-2.5">
                        <div className="font-medium">{s.username}</div>
                        <div className="text-[10px] text-slate-400 truncate max-w-[140px]">
                          {s.appName || s.clientAddr || 'local'}
                        </div>
                      </td>
                      <td className="px-3 py-2.5">
                        <span
                          className={`inline-flex px-2 py-0.5 rounded-full text-[10px] font-semibold ${
                            s.state === 'active'
                              ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300'
                              : s.state.includes('transaction')
                              ? 'bg-amber-500/15 text-amber-600 dark:text-amber-300'
                              : 'bg-slate-500/15 text-slate-500'
                          }`}
                        >
                          {s.state}
                        </span>
                        {s.waitEvent && (
                          <div className="text-[10px] text-amber-500 mt-0.5">{s.waitEvent}</div>
                        )}
                        {s.blockingPids && s.blockingPids.length > 0 && (
                          <div className="text-[10px] text-red-500 font-semibold mt-0.5">
                            Blocked by: {s.blockingPids.join(', ')}
                          </div>
                        )}
                      </td>
                      <td className="px-3 py-2.5 font-mono">
                        {s.durationMs >= 1000
                          ? `${(s.durationMs / 1000).toFixed(2)}s`
                          : `${s.durationMs}ms`}
                      </td>
                      <td className="px-3 py-2.5 max-w-md">
                        <button
                          type="button"
                          onClick={() => setInspectedQuery(s)}
                          className="text-left font-mono text-[11px] text-slate-700 dark:text-slate-200 truncate block max-w-md hover:text-indigo-500"
                          title="Click to inspect full query"
                        >
                          {s.query || '—'}
                        </button>
                      </td>
                      <td className="px-3 py-2.5 text-right whitespace-nowrap">
                        <div className="inline-flex items-center gap-1">
                          <button
                            type="button"
                            onClick={() => setPidConfirm({ session: s, terminate: false })}
                            title="Cancel current query (pg_cancel_backend)"
                            className="inline-flex items-center gap-1 px-2 py-1 rounded-lg text-[11px] font-medium border border-amber-500/30 text-amber-600 dark:text-amber-300 hover:bg-amber-500/10"
                          >
                            <XOctagon className="w-3 h-3" />
                            Cancel
                          </button>
                          <button
                            type="button"
                            onClick={() => setPidConfirm({ session: s, terminate: true })}
                            title="Terminate connection (pg_terminate_backend)"
                            className="inline-flex items-center gap-1 px-2 py-1 rounded-lg text-[11px] font-medium border border-red-500/30 text-red-600 dark:text-red-400 hover:bg-red-500/10"
                          >
                            <Skull className="w-3 h-3" />
                            Kill
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {tab === 'locks' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-950/60 border-b border-slate-200 dark:border-slate-800 text-[11px] uppercase tracking-wider text-slate-500">
                <tr>
                  <th className="px-3 py-2.5">PID</th>
                  <th className="px-3 py-2.5">Status</th>
                  <th className="px-3 py-2.5">Database</th>
                  <th className="px-3 py-2.5">Relation</th>
                  <th className="px-3 py-2.5">Lock Type / Mode</th>
                  <th className="px-3 py-2.5">Query</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
                {(data?.locks || []).length === 0 ? (
                  <tr>
                    <td colSpan={6} className="px-4 py-8 text-center text-slate-400">
                      No active user relation locks right now.
                    </td>
                  </tr>
                ) : (
                  (data?.locks || []).map((l, idx) => (
                    <tr key={`${l.pid}-${idx}`} className={!l.granted ? 'bg-red-500/5' : ''}>
                      <td className="px-3 py-2.5 font-mono font-semibold">{l.pid}</td>
                      <td className="px-3 py-2.5">
                        <span
                          className={`inline-flex px-2 py-0.5 rounded-full text-[10px] font-semibold ${
                            l.granted
                              ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300'
                              : 'bg-red-500/20 text-red-600 dark:text-red-300'
                          }`}
                        >
                          {l.granted ? 'GRANTED' : 'WAITING'}
                        </span>
                      </td>
                      <td className="px-3 py-2.5 font-mono">{l.database}</td>
                      <td className="px-3 py-2.5 font-mono">{l.relation || '—'}</td>
                      <td className="px-3 py-2.5 font-mono text-[11px]">
                        {l.lockType} &middot; {l.mode}
                      </td>
                      <td className="px-3 py-2.5 font-mono text-[11px] max-w-md truncate">
                        {l.query || '—'}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {tab === 'tables' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-950/60 border-b border-slate-200 dark:border-slate-800 text-[11px] uppercase tracking-wider text-slate-500">
                <tr>
                  <th className="px-3 py-2.5">Table</th>
                  <th className="px-3 py-2.5">Total Size</th>
                  <th className="px-3 py-2.5">Table / Indexes</th>
                  <th className="px-3 py-2.5">Live / Dead Tuples</th>
                  <th className="px-3 py-2.5">Seq vs Idx Scans</th>
                  <th className="px-3 py-2.5">Last Vacuum / Analyze</th>
                  <th className="px-3 py-2.5 text-right">Maintenance</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
                {(data?.tableStats || []).length === 0 ? (
                  <tr>
                    <td colSpan={7} className="px-4 py-8 text-center text-slate-400">
                      Select a database in DB Scope above to view table sizes and bloat counters.
                    </td>
                  </tr>
                ) : (
                  (data?.tableStats || []).map((t) => {
                    const highDead = t.deadTuples > 1000 && t.deadTuples > t.liveTuples * 0.2;
                    return (
                      <tr key={`${t.schema}.${t.table}`}>
                        <td className="px-3 py-2.5 font-mono font-semibold">
                          <span className="text-slate-400">{t.schema}.</span>
                          {t.table}
                        </td>
                        <td className="px-3 py-2.5 font-mono font-semibold">{t.totalSizePretty}</td>
                        <td className="px-3 py-2.5 font-mono text-[11px] text-slate-500">
                          {t.tableSizePretty} / {t.indexSizePretty}
                        </td>
                        <td className="px-3 py-2.5 font-mono">
                          <span>{t.liveTuples.toLocaleString()}</span>
                          {' / '}
                          <span className={highDead ? 'text-amber-500 font-bold' : 'text-slate-400'}>
                            {t.deadTuples.toLocaleString()} dead
                          </span>
                        </td>
                        <td className="px-3 py-2.5 font-mono text-[11px]">
                          <span className={t.seqScans > t.idxScans * 5 && t.liveTuples > 5000 ? 'text-amber-500' : ''}>
                            {t.seqScans.toLocaleString()} seq
                          </span>
                          {' / '}
                          <span className="text-emerald-600 dark:text-emerald-400">
                            {t.idxScans.toLocaleString()} idx
                          </span>
                        </td>
                        <td className="px-3 py-2.5 text-[11px] text-slate-500">
                          {t.lastVacuum ? `V: ${new Date(t.lastVacuum).toLocaleDateString()}` : 'V: never'}
                          {' · '}
                          {t.lastAnalyze ? `A: ${new Date(t.lastAnalyze).toLocaleDateString()}` : 'A: never'}
                        </td>
                        <td className="px-3 py-2.5 text-right whitespace-nowrap">
                          <div className="inline-flex items-center gap-1">
                            <button
                              type="button"
                              onClick={() => void handleMaintain(t.schema, t.table, 'vacuum_analyze')}
                              className="px-2 py-1 rounded-lg border border-indigo-500/30 text-indigo-600 dark:text-indigo-300 text-[11px] font-medium hover:bg-indigo-500/10"
                            >
                              Vacuum Analyze
                            </button>
                            <button
                              type="button"
                              onClick={() => void handleMaintain(t.schema, t.table, 'reindex')}
                              className="px-2 py-1 rounded-lg border border-slate-200 dark:border-slate-700 text-[11px] font-medium hover:bg-slate-100 dark:hover:bg-slate-800"
                            >
                              Reindex
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {tab === 'indexes' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-950/60 border-b border-slate-200 dark:border-slate-800 text-[11px] uppercase tracking-wider text-slate-500">
                <tr>
                  <th className="px-3 py-2.5">Schema.Table</th>
                  <th className="px-3 py-2.5">Index Name</th>
                  <th className="px-3 py-2.5">Size</th>
                  <th className="px-3 py-2.5">Scans</th>
                  <th className="px-3 py-2.5 text-right">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
                {(data?.unusedIndexes || []).length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-8 text-center text-slate-400">
                      No unused non-unique indexes detected!
                    </td>
                  </tr>
                ) : (
                  (data?.unusedIndexes || []).map((ix) => (
                    <tr key={`${ix.schema}.${ix.indexName}`}>
                      <td className="px-3 py-2.5 font-mono">
                        {ix.schema}.{ix.table}
                      </td>
                      <td className="px-3 py-2.5 font-mono font-semibold text-amber-600 dark:text-amber-400">
                        {ix.indexName}
                      </td>
                      <td className="px-3 py-2.5 font-mono">{ix.sizePretty}</td>
                      <td className="px-3 py-2.5 font-mono">0</td>
                      <td className="px-3 py-2.5 text-right">
                        <button
                          type="button"
                          onClick={() => setDropIdxConfirm(ix)}
                          className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-red-500/30 text-red-600 dark:text-red-400 text-[11px] font-medium hover:bg-red-500/10"
                        >
                          <Trash2 className="w-3 h-3" />
                          Drop Index
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Inspect Full Query Modal */}
      <Modal
        open={!!inspectedQuery}
        onClose={() => setInspectedQuery(null)}
        title={`Backend PID ${inspectedQuery?.pid ?? ''}`}
        subtitle={`${inspectedQuery?.username ?? ''} @ ${inspectedQuery?.database ?? ''} (${inspectedQuery?.state ?? ''})`}
        maxWidthClass="max-w-2xl"
      >
        <pre className="rounded-xl bg-slate-950 text-slate-100 p-4 text-xs font-mono whitespace-pre-wrap overflow-x-auto max-h-96">
          {inspectedQuery?.query || '(no SQL recorded)'}
        </pre>
      </Modal>

      {/* Cancel / Terminate PID Confirm Modal */}
      <ConfirmModal
        open={!!pidConfirm}
        onClose={() => setPidConfirm(null)}
        title={pidConfirm?.terminate ? 'Terminate Backend Connection' : 'Cancel Running Query'}
        subtitle={`PID ${pidConfirm?.session.pid ?? ''}`}
        message={
          pidConfirm?.terminate ? (
            <>
              Terminating PID <strong>{pidConfirm.session.pid}</strong> forcibly disconnects the
              client session and rolls back any uncommitted transaction.
            </>
          ) : (
            <>
              Send cancellation signal to active query on PID{' '}
              <strong>{pidConfirm?.session.pid}</strong>? The client connection stays open.
            </>
          )
        }
        confirmLabel={pidConfirm?.terminate ? 'Kill Connection' : 'Cancel Query'}
        loading={acting}
        onConfirm={handlePidAction}
      />

      {/* Drop Unused Index Confirm Modal */}
      <ConfirmModal
        open={!!dropIdxConfirm}
        onClose={() => setDropIdxConfirm(null)}
        title="Drop Unused Index"
        subtitle={`${dropIdxConfirm?.schema}.${dropIdxConfirm?.table}`}
        message={
          <>
            Drop index <strong>{dropIdxConfirm?.indexName}</strong> ({dropIdxConfirm?.sizePretty})?
          </>
        }
        confirmTextMatch={dropIdxConfirm?.indexName}
        confirmLabel="Drop Index"
        loading={acting}
        onConfirm={handleDropUnusedIndex}
      />
    </div>
  );
}

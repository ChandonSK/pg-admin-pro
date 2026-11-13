import {
  Activity,
  Archive,
  ArrowRight,
  Database,
  HardDrive,
  Plus,
  RefreshCw,
  Shield,
  Users,
} from 'lucide-react';
import type { DatabaseInfo, ServerOverview, UserInfo } from '../types';
import type { NavTab } from './Navbar';

interface OverviewPageProps {
  overview: ServerOverview;
  databases: DatabaseInfo[];
  users: UserInfo[];
  onNavigate: (tab: NavTab) => void;
  onEnterDatabase: (dbName: string) => void;
  onRefresh: () => void;
}

function formatUptime(seconds: number): string {
  if (!seconds || seconds <= 0) return '-';
  const days = Math.floor(seconds / 86400);
  const hrs = Math.floor((seconds % 86400) / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hrs}h`;
  if (hrs > 0) return `${hrs}h ${mins}m`;
  return `${mins}m`;
}

export function OverviewPage({
  overview,
  databases,
  users,
  onNavigate,
  onEnterDatabase,
  onRefresh,
}: OverviewPageProps) {
  const stats = [
    {
      label: 'Total Databases',
      value: String(overview.totalDatabases),
      sub: `${databases.filter((d) => d.allowConnect).length} connectable`,
      icon: Database,
      color: 'text-indigo-500 bg-indigo-500/10',
      onClick: () => onNavigate('databases'),
    },
    {
      label: 'Users & Roles',
      value: String(overview.totalUsers),
      sub: `${users.filter((u) => u.canLogin).length} login roles`,
      icon: Users,
      color: 'text-emerald-500 bg-emerald-500/10',
      onClick: () => onNavigate('users'),
    },
    {
      label: 'Total Storage',
      value: overview.totalSizePretty,
      sub: overview.engine.toUpperCase(),
      icon: HardDrive,
      color: 'text-amber-500 bg-amber-500/10',
      onClick: () => onNavigate('databases'),
    },
    {
      label: 'Active Connections',
      value: String(overview.activeConns),
      sub: `Uptime: ${formatUptime(overview.uptimeSeconds)}`,
      icon: Activity,
      color: 'text-cyan-500 bg-cyan-500/10',
    },
  ];

  return (
    <div className="space-y-6">
      {/* Header Banner */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 p-5">
        <div>
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-500/15 text-emerald-600 dark:text-emerald-400">
              <Shield className="w-3 h-3" />
              {overview.isAdmin ? 'Admin / Superuser' : 'Standard Role'}
            </span>
            <span className="text-xs font-mono text-slate-500 dark:text-slate-400">
              {overview.version}
            </span>
          </div>
          <h1 className="text-xl font-bold tracking-tight text-slate-900 dark:text-white mt-1.5">
            Cluster Overview
          </h1>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
            Connected as <code className="font-mono text-indigo-500">{overview.currentUser}</code> on{' '}
            <code className="font-mono">{overview.currentDatabase}</code>
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={onRefresh}
            className="inline-flex items-center gap-1.5 px-3 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs font-medium hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            Refresh
          </button>
          <button
            type="button"
            onClick={() => onNavigate('databases')}
            className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-sm transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
            Manage Databases
          </button>
          <button
            type="button"
            onClick={() => onNavigate('backups')}
            className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-xl border border-emerald-600/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/20 text-xs font-semibold transition-colors"
          >
            <Archive className="w-3.5 h-3.5" />
            Backup Studio
          </button>
        </div>
      </div>

      {/* Stat Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {stats.map((st) => {
          const Icon = st.icon;
          return (
            <div
              key={st.label}
              onClick={st.onClick}
              className={`rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 p-4 flex items-center justify-between ${
                st.onClick ? 'cursor-pointer hover:border-indigo-500/50 transition-colors' : ''
              }`}
            >
              <div>
                <p className="text-xs font-medium text-slate-500 dark:text-slate-400">
                  {st.label}
                </p>
                <p className="text-2xl font-bold tracking-tight text-slate-900 dark:text-white mt-1">
                  {st.value}
                </p>
                <p className="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">
                  {st.sub}
                </p>
              </div>
              <div className={`w-11 h-11 rounded-xl flex items-center justify-center ${st.color}`}>
                <Icon className="w-5 h-5" />
              </div>
            </div>
          );
        })}
      </div>

      {/* Quick Enter Databases + Users Split */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
          <div className="flex items-center justify-between px-5 py-3.5 border-b border-slate-200 dark:border-slate-800">
            <h2 className="text-sm font-semibold">Databases on Server</h2>
            <button
              type="button"
              onClick={() => onNavigate('databases')}
              className="text-xs text-indigo-500 hover:text-indigo-400 font-medium inline-flex items-center gap-1"
            >
              View all <ArrowRight className="w-3 h-3" />
            </button>
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800/80 max-h-[380px] overflow-y-auto">
            {databases.map((db) => (
              <div
                key={db.name}
                className="flex items-center justify-between gap-3 px-5 py-3 hover:bg-slate-50 dark:hover:bg-slate-800/40 transition-colors"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm font-semibold text-slate-900 dark:text-white truncate">
                      {db.name}
                    </span>
                    <span className="text-[11px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 font-mono">
                      {db.sizePretty}
                    </span>
                  </div>
                  <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5 truncate">
                    Owner: <span className="font-mono">{db.owner}</span> • Assigned:{' '}
                    {db.assignedUsers.length > 0 ? db.assignedUsers.join(', ') : db.owner}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={!db.allowConnect}
                  onClick={() => onEnterDatabase(db.name)}
                  className="shrink-0 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white text-xs font-medium transition-colors"
                >
                  Enter <ArrowRight className="w-3.5 h-3.5" />
                </button>
              </div>
            ))}
            {databases.length === 0 && (
              <div className="px-5 py-8 text-center text-xs text-slate-500">
                No databases found.
              </div>
            )}
          </div>
        </div>

        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
          <div className="flex items-center justify-between px-5 py-3.5 border-b border-slate-200 dark:border-slate-800">
            <h2 className="text-sm font-semibold">Database Users</h2>
            {overview.capabilities.userManagement && (
              <button
                type="button"
                onClick={() => onNavigate('users')}
                className="text-xs text-indigo-500 hover:text-indigo-400 font-medium inline-flex items-center gap-1"
              >
                Manage <ArrowRight className="w-3 h-3" />
              </button>
            )}
          </div>
          <div className="divide-y divide-slate-100 dark:divide-slate-800/80 max-h-[380px] overflow-y-auto">
            {users.map((u) => (
              <div key={u.name} className="px-5 py-3 flex items-center justify-between gap-2">
                <div className="min-w-0">
                  <p className="font-mono text-xs font-semibold text-slate-900 dark:text-white truncate">
                    {u.name}
                  </p>
                  <p className="text-[11px] text-slate-500 dark:text-slate-400 truncate mt-0.5">
                    {u.ownedDatabases.length > 0
                      ? `Owns: ${u.ownedDatabases.join(', ')}`
                      : 'No owned DBs'}
                  </p>
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  {u.isSuperuser && (
                    <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-amber-500/15 text-amber-600 dark:text-amber-300">
                      SUPER
                    </span>
                  )}
                  <span
                    className={`px-1.5 py-0.5 rounded text-[10px] font-semibold ${
                      u.canLogin
                        ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
                        : 'bg-slate-500/15 text-slate-500'
                    }`}
                  >
                    {u.canLogin ? 'LOGIN' : 'ROLE'}
                  </span>
                </div>
              </div>
            ))}
            {users.length === 0 && (
              <div className="px-5 py-8 text-center text-xs text-slate-500">
                User management not available for this engine.
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

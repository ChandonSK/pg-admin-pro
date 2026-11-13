import { useEffect, useState, type FormEvent } from 'react';
import {
  Activity,
  Archive,
  ArrowRightLeft,
  Bookmark,
  ChevronDown,
  Database,
  LayoutDashboard,
  Loader2,
  LogOut,
  Moon,
  Plus,
  Server,
  Sun,
  Trash2,
  Users,
} from 'lucide-react';
import { api, type SessionPayload } from '../api/client';
import type {
  DatabaseInfo,
  ProfileEnv,
  PublicConnectionInfo,
  ServerOverview,
  ServerProfile,
} from '../types';
import { ConfirmModal } from './ConfirmModal';
import { Modal } from './Modal';
import type { ToastItem } from './Toast';

export type NavTab =
  | 'overview'
  | 'databases'
  | 'users'
  | 'monitor'
  | 'backups'
  | 'workspace';

interface NavbarProps {
  activeTab: NavTab;
  onSelectTab: (tab: NavTab) => void;
  connection: PublicConnectionInfo;
  overview: ServerOverview;
  databases: DatabaseInfo[];
  selectedDatabase: string;
  onEnterDatabase: (dbName: string) => void;
  onSwitchSession: (next: SessionPayload, label: string) => void;
  onNotify: (kind: ToastItem['kind'], message: string) => void;
  onLogout: () => void;
  dark: boolean;
  onToggleTheme: () => void;
}

export function Navbar({
  activeTab,
  onSelectTab,
  connection,
  overview,
  databases,
  selectedDatabase,
  onEnterDatabase,
  onSwitchSession,
  onNotify,
  onLogout,
  dark,
  onToggleTheme,
}: NavbarProps) {
  const [switcherOpen, setSwitcherOpen] = useState(false);
  const [profiles, setProfiles] = useState<ServerProfile[]>([]);
  const [loadingProfiles, setLoadingProfiles] = useState(false);
  const [switchingId, setSwitchingId] = useState('');
  const [promptPassFor, setPromptPassFor] = useState<ServerProfile | null>(null);
  const [switchPassword, setSwitchPassword] = useState('');

  // Quick Save Current Server state
  const [saveOpen, setSaveOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [newEnv, setNewEnv] = useState<ProfileEnv>('local');
  const [newPassword, setNewPassword] = useState('');
  const [savingProfile, setSavingProfile] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ServerProfile | null>(null);

  const loadProfiles = async () => {
    setLoadingProfiles(true);
    try {
      const res = await api.profiles.list();
      setProfiles(res.profiles || []);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to load profiles');
    } finally {
      setLoadingProfiles(false);
    }
  };

  useEffect(() => {
    if (switcherOpen) {
      void loadProfiles();
      setSaveOpen(false);
      setPromptPassFor(null);
      setNewName(
        connection.engine === 'sqlite'
          ? 'Local SQLite'
          : `${connection.host}:${connection.port}`
      );
      setNewPassword('');
    }
  }, [switcherOpen]);

  const handleSwitch = async (p: ServerProfile, passOverride?: string) => {
    if (!p.hasPassword && p.engine !== 'sqlite' && passOverride === undefined) {
      setPromptPassFor(p);
      setSwitchPassword('');
      return;
    }
    setSwitchingId(p.id);
    try {
      const next = await api.profiles.switchTo(p.id, passOverride);
      setSwitcherOpen(false);
      setPromptPassFor(null);
      onSwitchSession(next, p.name);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to switch server');
    } finally {
      setSwitchingId('');
    }
  };

  const handleSaveCurrent = async (e: FormEvent) => {
    e.preventDefault();
    setSavingProfile(true);
    try {
      await api.profiles.create({
        name: newName.trim(),
        env: newEnv,
        engine: connection.engine,
        host: connection.host,
        port: connection.port,
        database: connection.database,
        username: connection.username,
        password: newPassword,
        savePassword: newPassword.length > 0,
        sslMode: connection.sslMode,
      });
      onNotify('success', `Saved server profile "${newName.trim()}"`);
      setSaveOpen(false);
      setNewPassword('');
      await loadProfiles();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to save profile');
    } finally {
      setSavingProfile(false);
    }
  };

  const handleDeleteProfile = async () => {
    if (!deleteTarget) return;
    try {
      await api.profiles.remove(deleteTarget.id);
      onNotify('success', `Removed profile "${deleteTarget.name}"`);
      setDeleteTarget(null);
      await loadProfiles();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to remove profile');
    }
  };

  const envBadge = (env: string) => {
    if (env === 'production') return 'bg-red-500/15 text-red-600 dark:text-red-300 border-red-500/30';
    if (env === 'staging') return 'bg-amber-500/15 text-amber-600 dark:text-amber-300 border-amber-500/30';
    if (env === 'local') return 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300 border-emerald-500/30';
    return 'bg-slate-500/15 text-slate-600 dark:text-slate-300 border-slate-500/30';
  };

  const navItems: {
    id: NavTab;
    label: string;
    icon: typeof LayoutDashboard;
    badge?: number;
    hidden?: boolean;
  }[] = [
    { id: 'overview', label: 'Overview', icon: LayoutDashboard },
    {
      id: 'databases',
      label: 'Databases',
      icon: Database,
      badge: databases.length || overview.totalDatabases,
    },
    {
      id: 'users',
      label: 'Users & Roles',
      icon: Users,
      badge: overview.totalUsers,
      hidden: !overview.capabilities.userManagement,
    },
    {
      id: 'monitor',
      label: 'Activity Monitor',
      icon: Activity,
      hidden: !overview.capabilities.activityMonitor,
    },
    { id: 'backups', label: 'Backups', icon: Archive },
  ];

  return (
    <header className="sticky top-0 z-30 border-b border-slate-200 dark:border-slate-800 bg-white/90 dark:bg-slate-950/90 backdrop-blur-md">
      <div className="max-w-[1600px] mx-auto px-4 h-14 flex items-center justify-between gap-3">
        <div className="flex items-center gap-4 min-w-0">
          <button
            type="button"
            onClick={() => onSelectTab('overview')}
            className="flex items-center gap-2 shrink-0 group"
          >
            <span className="w-8 h-8 rounded-lg bg-indigo-600 text-white flex items-center justify-center shadow-sm shadow-indigo-600/30">
              <Database className="w-4 h-4" />
            </span>
            <span className="font-bold tracking-tight text-sm text-slate-900 dark:text-white hidden sm:inline">
              pg-admin-pro
            </span>
          </button>

          <nav className="flex items-center gap-1 overflow-x-auto" aria-label="Main Navigation">
            {navItems
              .filter((i) => !i.hidden)
              .map((item) => {
                const Icon = item.icon;
                const active = activeTab === item.id;
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => onSelectTab(item.id)}
                    className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium whitespace-nowrap transition-colors ${
                      active
                        ? 'bg-indigo-600 text-white shadow-sm'
                        : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/80'
                    }`}
                  >
                    <Icon className="w-3.5 h-3.5" />
                    <span>{item.label}</span>
                    {item.badge !== undefined && (
                      <span
                        className={`ml-0.5 px-1.5 py-0.5 rounded-full text-[10px] font-semibold ${
                          active
                            ? 'bg-white/20 text-white'
                            : 'bg-slate-200 dark:bg-slate-800 text-slate-700 dark:text-slate-300'
                        }`}
                      >
                        {item.badge}
                      </span>
                    )}
                  </button>
                );
              })}
          </nav>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {databases.length > 0 && (
            <div className="hidden xl:flex items-center gap-1.5">
              <label
                htmlFor="quick-db-jump"
                className="text-[11px] text-slate-500 dark:text-slate-400"
              >
                Enter DB:
              </label>
              <select
                id="quick-db-jump"
                value={activeTab === 'workspace' ? selectedDatabase : ''}
                onChange={(e) => {
                  if (e.target.value) onEnterDatabase(e.target.value);
                }}
                className="rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900 px-2.5 py-1 text-xs font-mono text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-indigo-500"
              >
                <option value="">Select database...</option>
                {databases
                  .filter((d) => d.allowConnect)
                  .map((d) => (
                    <option key={d.name} value={d.name}>
                      {d.name} ({d.sizePretty})
                    </option>
                  ))}
              </select>
            </div>
          )}

          <button
            type="button"
            onClick={() => setSwitcherOpen(true)}
            title="Switch saved server profile or bookmark this connection"
            className="hidden sm:flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-slate-100 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 hover:border-indigo-400 text-[11px] font-mono text-slate-700 dark:text-slate-200 transition-colors"
          >
            <span className="w-2 h-2 rounded-full bg-emerald-500" />
            <span className="truncate max-w-[180px]">
              {connection.engine === 'sqlite'
                ? connection.database
                : `${connection.username}@${connection.host}${connection.port > 0 ? `:${connection.port}` : ''}`}
            </span>
            <ChevronDown className="w-3 h-3 text-slate-400" />
          </button>

          <button
            type="button"
            onClick={onToggleTheme}
            aria-label="Toggle theme"
            className="p-2 rounded-lg text-slate-500 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
          >
            {dark ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
          </button>

          <button
            type="button"
            onClick={onLogout}
            className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium text-red-600 dark:text-red-400 hover:bg-red-500/10 transition-colors"
          >
            <LogOut className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">Disconnect</span>
          </button>
        </div>
      </div>

      {/* Multi-Server Switcher & Bookmark Modal */}
      <Modal
        open={switcherOpen}
        onClose={() => setSwitcherOpen(false)}
        title="Server Profiles & Quick Switcher"
        subtitle={`Connected to ${connection.engine} — ${connection.host || connection.database}`}
        maxWidthClass="max-w-xl"
      >
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-slate-500 uppercase tracking-wider flex items-center gap-1.5">
              <Bookmark className="w-3.5 h-3.5 text-indigo-500" />
              Saved Profiles
            </span>
            <button
              type="button"
              onClick={() => setSaveOpen((v) => !v)}
              className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium"
            >
              <Plus className="w-3.5 h-3.5" />
              Save Current Connection
            </button>
          </div>

          {saveOpen && (
            <form
              onSubmit={handleSaveCurrent}
              className="rounded-xl border border-indigo-500/30 bg-indigo-500/5 p-3.5 space-y-2.5"
            >
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                <input
                  type="text"
                  required
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="Profile label"
                  className="sm:col-span-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-1.5 text-xs"
                />
                <select
                  value={newEnv}
                  onChange={(e) => setNewEnv(e.target.value as ProfileEnv)}
                  className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-1.5 text-xs"
                >
                  <option value="local">Local</option>
                  <option value="staging">Staging</option>
                  <option value="production">Production</option>
                  <option value="other">Other</option>
                </select>
              </div>
              {connection.engine !== 'sqlite' && (
                <input
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder="Optional password to encrypt on server for 1-click switch"
                  className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-1.5 text-xs"
                />
              )}
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setSaveOpen(false)}
                  className="px-3 py-1 rounded-lg border border-slate-200 dark:border-slate-700 text-xs"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={savingProfile}
                  className="inline-flex items-center gap-1 px-3 py-1 rounded-lg bg-indigo-600 text-white text-xs font-semibold"
                >
                  {savingProfile && <Loader2 className="w-3 h-3 animate-spin" />}
                  Save Profile
                </button>
              </div>
            </form>
          )}

          {promptPassFor && (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void handleSwitch(promptPassFor, switchPassword);
              }}
              className="rounded-xl border border-amber-500/40 bg-amber-500/10 p-3.5 space-y-2.5"
            >
              <p className="text-xs font-medium text-amber-800 dark:text-amber-200">
                Enter password to switch to <strong>{promptPassFor.name}</strong> ({promptPassFor.username}@{promptPassFor.host}):
              </p>
              <div className="flex gap-2">
                <input
                  type="password"
                  autoFocus
                  required
                  value={switchPassword}
                  onChange={(e) => setSwitchPassword(e.target.value)}
                  placeholder="Server password"
                  className="flex-1 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs"
                />
                <button
                  type="submit"
                  disabled={!!switchingId}
                  className="px-3 py-1.5 rounded-lg bg-indigo-600 text-white text-xs font-semibold"
                >
                  Connect
                </button>
              </div>
            </form>
          )}

          {loadingProfiles ? (
            <div className="py-8 flex items-center justify-center text-xs text-slate-400 gap-2">
              <Loader2 className="w-4 h-4 animate-spin" />
              Loading profiles...
            </div>
          ) : profiles.length === 0 ? (
            <div className="py-8 text-center text-xs text-slate-400 border border-dashed border-slate-200 dark:border-slate-800 rounded-xl">
              No saved server profiles yet. Click &ldquo;Save Current Connection&rdquo; above to bookmark this server.
            </div>
          ) : (
            <div className="space-y-2 max-h-72 overflow-y-auto">
              {profiles.map((p) => (
                <div
                  key={p.id}
                  className="flex items-center justify-between gap-3 rounded-xl border border-slate-200 dark:border-slate-800 p-3 hover:border-indigo-500/40 transition-colors"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <Server className="w-3.5 h-3.5 text-indigo-500 shrink-0" />
                      <span className="font-semibold text-xs truncate">{p.name}</span>
                      <span
                        className={`px-1.5 py-0.5 rounded border text-[9px] font-bold uppercase ${envBadge(p.env)}`}
                      >
                        {p.env}
                      </span>
                    </div>
                    <p className="text-[11px] font-mono text-slate-500 mt-0.5 truncate">
                      {p.engine} &middot;{' '}
                      {p.engine === 'sqlite' ? p.database : `${p.username}@${p.host}:${p.port}/${p.database}`}
                    </p>
                  </div>

                  <div className="flex items-center gap-1.5 shrink-0">
                    <button
                      type="button"
                      disabled={switchingId === p.id}
                      onClick={() => void handleSwitch(p)}
                      className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium"
                    >
                      {switchingId === p.id ? (
                        <Loader2 className="w-3 h-3 animate-spin" />
                      ) : (
                        <ArrowRightLeft className="w-3 h-3" />
                      )}
                      Switch
                    </button>
                    <button
                      type="button"
                      onClick={() => setDeleteTarget(p)}
                      aria-label={`Delete profile ${p.name}`}
                      className="p-1.5 rounded-lg text-slate-400 hover:text-red-500 hover:bg-red-500/10"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </Modal>

      <ConfirmModal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title="Delete Server Profile"
        message={
          <>
            Remove saved connection bookmark <strong>{deleteTarget?.name}</strong>?
          </>
        }
        confirmLabel="Remove Profile"
        onConfirm={handleDeleteProfile}
      />
    </header>
  );
}

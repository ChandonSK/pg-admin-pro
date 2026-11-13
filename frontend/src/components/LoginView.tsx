import { useEffect, useState, type FormEvent } from 'react';
import {
  Bookmark,
  Database,
  Eye,
  EyeOff,
  KeyRound,
  Link2,
  Loader2,
  Moon,
  Server,
  ShieldCheck,
  Sun,
  Zap,
} from 'lucide-react';
import { api, type SessionPayload } from '../api/client';
import type { EngineKind, ProfileEnv, ServerProfile } from '../types';

interface LoginViewProps {
  onSuccess: (session: SessionPayload) => void;
  dark: boolean;
  onToggleTheme: () => void;
}

const DEFAULT_PORTS: Record<EngineKind, number> = {
  postgres: 5432,
  mysql: 3306,
  sqlite: 0,
};

export function LoginView({ onSuccess, dark, onToggleTheme }: LoginViewProps) {
  const [engine, setEngine] = useState<EngineKind>('postgres');
  const [host, setHost] = useState('127.0.0.1');
  const [port, setPort] = useState(5432);
  const [database, setDatabase] = useState('postgres');
  const [username, setUsername] = useState('postgres');
  const [password, setPassword] = useState('');
  const [sslMode, setSslMode] = useState('disable');
  const [uriInput, setUriInput] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [profiles, setProfiles] = useState<ServerProfile[]>([]);
  const [selectedProfileId, setSelectedProfileId] = useState<string>('');
  const [saveAsProfile, setSaveAsProfile] = useState(false);
  const [profileName, setProfileName] = useState('');
  const [profileEnv, setProfileEnv] = useState<ProfileEnv>('local');
  const [savePassword, setSavePassword] = useState(false);

  useEffect(() => {
    api.auth
      .defaults()
      .then((d) => {
        if (d.engine) setEngine(d.engine);
        if (d.host) setHost(d.host);
        if (d.port) setPort(d.port);
        if (d.database) setDatabase(d.database);
        if (d.username) setUsername(d.username);
        if (d.sslMode) setSslMode(d.sslMode);
      })
      .catch(() => {});
    api.profiles
      .list()
      .then((res) => setProfiles(res.profiles || []))
      .catch(() => {});
  }, []);

  const handleEngineSwitch = (next: EngineKind) => {
    setEngine(next);
    setError('');
    if (next === 'postgres') {
      setPort(DEFAULT_PORTS.postgres);
      if (!database || database.endsWith('.db')) setDatabase('postgres');
      if (!username) setUsername('postgres');
      setSslMode('disable');
    } else if (next === 'mysql') {
      setPort(DEFAULT_PORTS.mysql);
      if (database === 'postgres' || database.endsWith('.db')) setDatabase('');
      if (username === 'postgres') setUsername('root');
      setSslMode('false');
    } else {
      setPort(0);
      if (database === 'postgres' || database === '') setDatabase('./data.db');
    }
  };

  const applyUri = (raw: string) => {
    setUriInput(raw);
    const trimmed = raw.trim();
    if (!trimmed) return;
    try {
      const normalized = trimmed.replace(/^postgresql:\/\//i, 'postgres://');
      const u = new URL(normalized);
      if (u.protocol === 'postgres:') {
        setEngine('postgres');
        setHost(u.hostname || '127.0.0.1');
        setPort(u.port ? Number(u.port) : 5432);
        setUsername(decodeURIComponent(u.username || 'postgres'));
        setPassword(decodeURIComponent(u.password || ''));
        setDatabase(decodeURIComponent(u.pathname.replace(/^\//, '')) || 'postgres');
        setSslMode(u.searchParams.get('sslmode') || 'disable');
        setError('');
      } else if (u.protocol === 'mysql:') {
        setEngine('mysql');
        setHost(u.hostname || '127.0.0.1');
        setPort(u.port ? Number(u.port) : 3306);
        setUsername(decodeURIComponent(u.username || 'root'));
        setPassword(decodeURIComponent(u.password || ''));
        setDatabase(decodeURIComponent(u.pathname.replace(/^\//, '')));
        setError('');
      }
    } catch {
      // Wait for user to finish typing URI
    }
  };

  const selectProfile = (p: ServerProfile) => {
    setSelectedProfileId(p.id);
    setEngine(p.engine);
    setHost(p.host);
    setPort(p.port);
    setDatabase(p.database);
    setUsername(p.username);
    setSslMode(p.sslMode || 'disable');
    setPassword('');
    setSaveAsProfile(false);
    setError('');
  };

  const quickConnectProfile = async (p: ServerProfile) => {
    selectProfile(p);
    if (!p.hasPassword && p.engine !== 'sqlite') {
      return;
    }
    setLoading(true);
    try {
      const session = await api.auth.login({
        profileId: p.id,
        password: '',
      });
      onSuccess(session);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect profile');
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const session = await api.auth.login(
        selectedProfileId && password === ''
          ? { profileId: selectedProfileId, password: '' }
          : {
              engine,
              host: host.trim(),
              port: Number(port) || 0,
              database: database.trim(),
              username: username.trim(),
              password,
              sslMode,
              saveProfileName: saveAsProfile ? profileName.trim() : undefined,
              saveProfileEnv: saveAsProfile ? profileEnv : undefined,
              savePassword: saveAsProfile ? savePassword : false,
            }
      );
      onSuccess(session);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to connect');
    } finally {
      setLoading(false);
    }
  };

  const envBadge = (env: string) => {
    if (env === 'production') return 'bg-red-500/15 text-red-600 dark:text-red-300 border-red-500/30';
    if (env === 'staging') return 'bg-amber-500/15 text-amber-600 dark:text-amber-300 border-amber-500/30';
    if (env === 'local') return 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300 border-emerald-500/30';
    return 'bg-slate-500/15 text-slate-600 dark:text-slate-300 border-slate-500/30';
  };

  return (
    <div className="min-h-screen flex flex-col justify-center items-center px-4 py-10 bg-slate-50 dark:bg-slate-950 relative overflow-hidden">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-indigo-500/15 via-transparent to-transparent"
      />

      <button
        type="button"
        onClick={onToggleTheme}
        aria-label="Toggle color theme"
        className="fixed top-4 right-4 p-2.5 rounded-xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/80 text-slate-600 dark:text-slate-300 hover:text-indigo-500 transition-colors"
      >
        {dark ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
      </button>

      <div className="w-full max-w-lg relative z-10">
        <div className="text-center mb-6">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-2xl bg-indigo-600 text-white shadow-lg shadow-indigo-600/30 mb-3">
            <Database className="w-6 h-6" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900 dark:text-white">
            pg-admin-pro
          </h1>
          <p className="text-sm text-slate-600 dark:text-slate-400 mt-1">
            High-speed Database Administration &amp; Zero-Conflict Backup Studio
          </p>
        </div>

        {profiles.length > 0 && (
          <div className="mb-4 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/95 p-3.5 shadow-lg">
            <div className="flex items-center justify-between mb-2">
              <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 flex items-center gap-1.5">
                <Bookmark className="w-3.5 h-3.5 text-indigo-500" />
                Saved Server Profiles
              </span>
              {selectedProfileId && (
                <button
                  type="button"
                  onClick={() => setSelectedProfileId('')}
                  className="text-[11px] text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
                >
                  Clear selection
                </button>
              )}
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-44 overflow-y-auto pr-0.5">
              {profiles.map((p) => {
                const selected = selectedProfileId === p.id;
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => void quickConnectProfile(p)}
                    className={`text-left rounded-xl border p-2.5 transition-all ${
                      selected
                        ? 'border-indigo-500 bg-indigo-500/10'
                        : 'border-slate-200 dark:border-slate-800 hover:border-indigo-400 bg-slate-50/70 dark:bg-slate-950/60'
                    }`}
                  >
                    <div className="flex items-center justify-between gap-1.5">
                      <span className="font-semibold text-xs text-slate-900 dark:text-white truncate">
                        {p.name}
                      </span>
                      <span className={`px-1.5 py-0.5 rounded border text-[9px] font-bold uppercase ${envBadge(p.env)}`}>
                        {p.env}
                      </span>
                    </div>
                    <div className="mt-1 flex items-center justify-between gap-1 text-[10px] font-mono text-slate-500">
                      <span className="truncate">
                        {p.engine === 'sqlite' ? p.database : `${p.username}@${p.host}:${p.port}`}
                      </span>
                      {(p.hasPassword || p.engine === 'sqlite') && (
                        <span className="inline-flex items-center gap-0.5 text-emerald-500 shrink-0 font-sans font-semibold">
                          <Zap className="w-2.5 h-2.5" />
                          1-Click
                        </span>
                      )}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>
        )}

        <form
          onSubmit={handleSubmit}
          className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/95 shadow-2xl p-6 space-y-5"
        >
          {/* Engine selector */}
          <div>
            <span className="block text-xs font-medium uppercase tracking-wider text-slate-500 dark:text-slate-400 mb-2">
              Database Engine
            </span>
            <div className="grid grid-cols-3 gap-2">
              {(
                [
                  { id: 'postgres', label: 'PostgreSQL' },
                  { id: 'mysql', label: 'MySQL / Maria' },
                  { id: 'sqlite', label: 'SQLite File' },
                ] as { id: EngineKind; label: string }[]
              ).map((item) => (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => handleEngineSwitch(item.id)}
                  className={`py-2 px-3 rounded-xl text-xs font-semibold border transition-all ${
                    engine === item.id
                      ? 'bg-indigo-600 border-indigo-500 text-white shadow-sm shadow-indigo-600/25'
                      : 'bg-slate-50 dark:bg-slate-800/70 border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-300 hover:border-indigo-400'
                  }`}
                >
                  {item.label}
                </button>
              ))}
            </div>
          </div>

          {engine !== 'sqlite' && (
            <div>
              <label
                htmlFor="conn-uri"
                className="flex items-center gap-1.5 text-xs font-medium text-slate-600 dark:text-slate-400 mb-1.5"
              >
                <Link2 className="w-3.5 h-3.5" />
                Quick-Fill from Connection URI (optional)
              </label>
              <input
                id="conn-uri"
                type="text"
                value={uriInput}
                onChange={(e) => applyUri(e.target.value)}
                placeholder="postgres://postgres:password@127.0.0.1:5432/postgres?sslmode=disable"
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
          )}

          {engine !== 'sqlite' ? (
            <>
              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <label
                    htmlFor="login-host"
                    className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1"
                  >
                    Host
                  </label>
                  <div className="relative">
                    <Server className="w-4 h-4 text-slate-400 absolute left-3 top-2.5" />
                    <input
                      id="login-host"
                      type="text"
                      required
                      value={host}
                      onChange={(e) => setHost(e.target.value)}
                      className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 pl-9 pr-3 py-2 text-sm text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                    />
                  </div>
                </div>
                <div>
                  <label
                    htmlFor="login-port"
                    className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1"
                  >
                    Port
                  </label>
                  <input
                    id="login-port"
                    type="number"
                    required
                    value={port}
                    onChange={(e) => setPort(Number(e.target.value))}
                    className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm font-mono text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label
                    htmlFor="login-db"
                    className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1"
                  >
                    Initial Database
                  </label>
                  <input
                    id="login-db"
                    type="text"
                    value={database}
                    onChange={(e) => setDatabase(e.target.value)}
                    placeholder={engine === 'postgres' ? 'postgres' : 'optional'}
                    className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  />
                </div>
                <div>
                  <label
                    htmlFor="login-ssl"
                    className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1"
                  >
                    SSL Mode
                  </label>
                  <select
                    id="login-ssl"
                    value={sslMode}
                    onChange={(e) => setSslMode(e.target.value)}
                    className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  >
                    <option value="disable">disable</option>
                    <option value="prefer">prefer</option>
                    <option value="require">require</option>
                    <option value="verify-full">verify-full</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label
                    htmlFor="login-user"
                    className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1"
                  >
                    Admin Username
                  </label>
                  <input
                    id="login-user"
                    type="text"
                    required
                    autoComplete="username"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                  />
                </div>
                <div>
                  <label
                    htmlFor="login-pass"
                    className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1"
                  >
                    Password
                  </label>
                  <div className="relative">
                    <input
                      id="login-pass"
                      type={showPassword ? 'text' : 'password'}
                      autoComplete="current-password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      placeholder="••••••••"
                      className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 pl-3 pr-9 py-2 text-sm text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                    />
                    <button
                      type="button"
                      onClick={() => setShowPassword((v) => !v)}
                      aria-label={showPassword ? 'Hide password' : 'Show password'}
                      className="absolute right-2.5 top-2.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
                    >
                      {showPassword ? (
                        <EyeOff className="w-4 h-4" />
                      ) : (
                        <Eye className="w-4 h-4" />
                      )}
                    </button>
                  </div>
                </div>
              </div>
            </>
          ) : (
            <div>
              <label
                htmlFor="sqlite-path"
                className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1"
              >
                SQLite Database File Path
              </label>
              <input
                id="sqlite-path"
                type="text"
                required
                value={database}
                onChange={(e) => setDatabase(e.target.value)}
                placeholder="/var/lib/app/database.db"
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm font-mono text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
          )}

          {/* Save as Server Profile */}
          <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50/70 dark:bg-slate-950/60 p-3 space-y-2.5">
            <label className="flex items-center gap-2 text-xs font-medium text-slate-700 dark:text-slate-300 cursor-pointer">
              <input
                type="checkbox"
                checked={saveAsProfile}
                onChange={(e) => {
                  setSaveAsProfile(e.target.checked);
                  if (e.target.checked && !profileName) {
                    setProfileName(engine === 'sqlite' ? 'Local SQLite' : `${host}:${port}`);
                  }
                }}
                className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
              />
              Save this connection as a reusable server profile
            </label>
            {saveAsProfile && (
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 pt-1">
                <input
                  type="text"
                  required
                  value={profileName}
                  onChange={(e) => setProfileName(e.target.value)}
                  placeholder="Profile name (e.g. VPS Prod)"
                  className="sm:col-span-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-1.5 text-xs focus:outline-none focus:ring-2 focus:ring-indigo-500"
                />
                <select
                  value={profileEnv}
                  onChange={(e) => setProfileEnv(e.target.value as ProfileEnv)}
                  className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-1.5 text-xs"
                >
                  <option value="local">Local</option>
                  <option value="staging">Staging</option>
                  <option value="production">Production</option>
                  <option value="other">Other</option>
                </select>
                {engine !== 'sqlite' && (
                  <label className="sm:col-span-3 flex items-center gap-2 text-[11px] text-slate-500 dark:text-slate-400 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={savePassword}
                      onChange={(e) => setSavePassword(e.target.checked)}
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
                    />
                    Save password encrypted on server (AES-256-GCM) for 1-click switching
                  </label>
                )}
              </div>
            )}
          </div>

          {error && (
            <div
              role="alert"
              className="rounded-xl border border-red-500/40 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-600 dark:text-red-300"
            >
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full flex items-center justify-center gap-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 disabled:opacity-60 text-white font-semibold py-2.5 px-4 text-sm shadow-lg shadow-indigo-600/25 transition-all"
          >
            {loading ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                Connecting to {engine}...
              </>
            ) : (
              <>
                <KeyRound className="w-4 h-4" />
                Connect &amp; Open Studio
              </>
            )}
          </button>

          <div className="flex items-center justify-center gap-1.5 text-[11px] text-slate-500 dark:text-slate-400 pt-1">
            <ShieldCheck className="w-3.5 h-3.5 text-emerald-500" />
            Credentials stay in server memory only via HTTP-only session cookie
          </div>
        </form>
      </div>
    </div>
  );
}

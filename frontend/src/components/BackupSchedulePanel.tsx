import type { FormEvent } from 'react';
import {
  AlertTriangle,
  CalendarClock,
  CheckCircle2,
  Loader2,
  Play,
  Scissors,
  Save,
} from 'lucide-react';
import type {
  BackupSettingsUpdateReq,
  BackupSettingsView,
  DatabaseInfo,
  EngineKind,
} from '../types';

interface BackupSchedulePanelProps {
  view: BackupSettingsView;
  form: BackupSettingsUpdateReq;
  engine: EngineKind;
  databases: DatabaseInfo[];
  saving: boolean;
  runningNow: boolean;
  pruning: boolean;
  onChange: (patch: Partial<BackupSettingsUpdateReq>) => void;
  onSave: () => Promise<void>;
  onRunNow: () => Promise<void>;
  onPruneNow: () => Promise<void>;
}

export function BackupSchedulePanel({
  view,
  form,
  engine,
  databases,
  saving,
  runningNow,
  pruning,
  onChange,
  onSave,
  onRunNow,
  onPruneNow,
}: BackupSchedulePanelProps) {
  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    await onSave();
  };

  const statusTone =
    view.lastRunStatus === 'ok'
      ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
      : view.lastRunStatus === 'partial'
        ? 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
        : 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300';

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 p-5 space-y-5 shadow-sm"
    >
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-bold flex items-center gap-2">
            <CalendarClock className="w-4 h-4 text-indigo-500" />
            Automated Daily Schedule &amp; Retention Policy
          </h2>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
            Runs daily snapshots once per day after the configured UTC/local server clock time and prunes old files automatically.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            disabled={pruning}
            onClick={() => void onPruneNow()}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium disabled:opacity-50"
          >
            {pruning ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Scissors className="w-3.5 h-3.5" />}
            Prune Old Backups
          </button>
          <button
            type="button"
            disabled={runningNow}
            onClick={() => void onRunNow()}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-emerald-600/40 bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 text-xs font-semibold disabled:opacity-50"
          >
            {runningNow ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
            Run Scheduled Now
          </button>
        </div>
      </div>

      {view.lastRunStatus && (
        <div className={`rounded-xl border px-3.5 py-2.5 text-xs flex items-start gap-2 ${statusTone}`}>
          {view.lastRunStatus === 'ok' ? (
            <CheckCircle2 className="w-4 h-4 shrink-0 mt-0.5" />
          ) : (
            <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
          )}
          <div className="min-w-0">
            <div className="font-semibold">
              Last Run ({view.lastRunStatus.toUpperCase()})
              {view.lastRunAt && (
                <span className="ml-2 font-mono font-normal opacity-80">
                  {new Date(view.lastRunAt).toLocaleString()}
                </span>
              )}
            </div>
            {view.lastRunMessage && (
              <div className="text-[11px] opacity-90 break-words mt-0.5">{view.lastRunMessage}</div>
            )}
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="flex items-center justify-between rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950/60 px-3.5 py-2.5">
          <div>
            <div className="text-xs font-semibold">Enable Daily Schedule</div>
            <div className="text-[11px] text-slate-500">Background worker check every minute</div>
          </div>
          <input
            id="auto-backup-enabled"
            type="checkbox"
            checked={form.autoBackupEnabled}
            onChange={(e) => onChange({ autoBackupEnabled: e.target.checked })}
            className="h-4 w-4 accent-indigo-600 cursor-pointer"
          />
        </div>

        <div>
          <label htmlFor="auto-backup-time" className="block text-[11px] font-medium text-slate-500 mb-1">
            Daily Run Time (24h HH:MM)
          </label>
          <input
            id="auto-backup-time"
            type="time"
            value={form.autoBackupTime}
            onChange={(e) => onChange({ autoBackupTime: e.target.value || '02:00' })}
            className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
          />
        </div>

        <div>
          <label htmlFor="auto-backup-comp" className="block text-[11px] font-medium text-slate-500 mb-1">
            Snapshot Compression (-Z 0..9)
          </label>
          <select
            id="auto-backup-comp"
            value={form.compression}
            onChange={(e) => onChange({ compression: Number(e.target.value) })}
            className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
          >
            <option value={6}>6 (Balanced)</option>
            <option value={9}>9 (Max Compression)</option>
            <option value={3}>3 (Fast)</option>
            <option value={0}>0 (Uncompressed)</option>
          </select>
        </div>

        <div>
          <label htmlFor="auto-backup-dbs" className="block text-[11px] font-medium text-slate-500 mb-1">
            Target Databases (<code className="font-mono">*</code> = all, or comma-separated)
          </label>
          <input
            id="auto-backup-dbs"
            type="text"
            list="auto-db-suggestions"
            value={form.autoBackupDatabase}
            onChange={(e) => onChange({ autoBackupDatabase: e.target.value })}
            placeholder="* or db1,db2"
            className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
          />
          <datalist id="auto-db-suggestions">
            <option value="*" />
            {databases.filter((d) => d.allowConnect).map((d) => (
              <option key={d.name} value={d.name} />
            ))}
          </datalist>
        </div>

        <div>
          <label htmlFor="retention-days" className="block text-[11px] font-medium text-slate-500 mb-1">
            Retention Days (0 = keep forever)
          </label>
          <input
            id="retention-days"
            type="number"
            min={0}
            value={form.retentionDays}
            onChange={(e) => onChange({ retentionDays: Math.max(0, Number(e.target.value) || 0) })}
            className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
          />
        </div>

        <div>
          <label htmlFor="max-backups" className="block text-[11px] font-medium text-slate-500 mb-1">
            Max Snapshots Kept (0 = unlimited)
          </label>
          <input
            id="max-backups"
            type="number"
            min={0}
            value={form.maxBackups}
            onChange={(e) => onChange({ maxBackups: Math.max(0, Number(e.target.value) || 0) })}
            className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
          />
        </div>
      </div>

      {/* Background PostgreSQL connection credentials */}
      <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50/70 dark:bg-slate-950/50 p-4 space-y-3">
        <div>
          <h3 className="text-xs font-semibold">
            Unattended Background PostgreSQL Connection {engine !== 'postgres' && '(PostgreSQL target)'}
          </h3>
          <p className="text-[11px] text-slate-500">
            Required for unattended daily timer runs when no browser session is active. Stored encrypted with AES-256-GCM.
          </p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label htmlFor="auto-host" className="block text-[11px] font-medium text-slate-500 mb-1">
              Host
            </label>
            <input
              id="auto-host"
              type="text"
              value={form.autoHost}
              onChange={(e) => onChange({ autoHost: e.target.value })}
              placeholder="127.0.0.1"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="auto-port" className="block text-[11px] font-medium text-slate-500 mb-1">
              Port
            </label>
            <input
              id="auto-port"
              type="number"
              min={1}
              max={65535}
              value={form.autoPort}
              onChange={(e) => onChange({ autoPort: Number(e.target.value) || 5432 })}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="auto-maint-db" className="block text-[11px] font-medium text-slate-500 mb-1">
              Maintenance DB
            </label>
            <input
              id="auto-maint-db"
              type="text"
              value={form.autoDatabase}
              onChange={(e) => onChange({ autoDatabase: e.target.value })}
              placeholder="postgres"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="auto-username" className="block text-[11px] font-medium text-slate-500 mb-1">
              Username
            </label>
            <input
              id="auto-username"
              type="text"
              value={form.autoUsername}
              onChange={(e) => onChange({ autoUsername: e.target.value })}
              placeholder="postgres"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="auto-password" className="block text-[11px] font-medium text-slate-500 mb-1">
              Password {view.hasAutoPassword && <span className="text-emerald-500">(saved)</span>}
            </label>
            <input
              id="auto-password"
              type="password"
              value={form.autoPassword || ''}
              onChange={(e) => onChange({ autoPassword: e.target.value, clearAutoPassword: false })}
              placeholder={view.hasAutoPassword ? 'Leave blank to keep existing' : 'Enter password'}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono"
            />
            {view.hasAutoPassword && (
              <label className="mt-1 inline-flex items-center gap-1.5 text-[11px] text-slate-500 cursor-pointer">
                <input
                  type="checkbox"
                  checked={Boolean(form.clearAutoPassword)}
                  onChange={(e) =>
                    onChange({
                      clearAutoPassword: e.target.checked,
                      autoPassword: e.target.checked ? '' : form.autoPassword,
                    })
                  }
                />
                Clear saved password
              </label>
            )}
          </div>

          <div>
            <label htmlFor="auto-sslmode" className="block text-[11px] font-medium text-slate-500 mb-1">
              SSL Mode
            </label>
            <select
              id="auto-sslmode"
              value={form.autoSslMode}
              onChange={(e) => onChange({ autoSslMode: e.target.value })}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-1.5 text-xs font-mono"
            >
              <option value="disable">disable</option>
              <option value="prefer">prefer</option>
              <option value="require">require</option>
              <option value="verify-full">verify-full</option>
            </select>
          </div>
        </div>
      </div>

      <div className="flex justify-end">
        <button
          type="submit"
          disabled={saving}
          className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold shadow-sm"
        >
          {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
          Save Schedule &amp; Retention Policy
        </button>
      </div>
    </form>
  );
}

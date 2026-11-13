import { useCallback, useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import {
  AlertTriangle,
  Archive,
  CalendarClock,
  CheckCircle2,
  Cloud,
  CloudUpload,
  Download,
  Loader2,
  Play,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Trash2,
  Upload,
} from 'lucide-react';
import { api } from '../api/client';
import type {
  BackupFile,
  BackupSettingsUpdateReq,
  BackupSettingsView,
  DatabaseInfo,
  S3Object,
  ServerOverview,
  ToolCapabilities,
} from '../types';
import { BackupS3Panel } from './BackupS3Panel';
import { BackupSchedulePanel } from './BackupSchedulePanel';
import { ConfirmModal } from './ConfirmModal';
import { Modal } from './Modal';

interface BackupsPageProps {
  overview: ServerOverview;
  databases: DatabaseInfo[];
  onRefreshCluster: () => void;
  onNotify: (kind: 'success' | 'error' | 'info', msg: string) => void;
}

type BackupTab = 'snapshots' | 'schedule' | 's3';

function toUpdateReq(v: BackupSettingsView): BackupSettingsUpdateReq {
  return {
    autoBackupEnabled: v.autoBackupEnabled,
    autoBackupTime: v.autoBackupTime || '02:00',
    autoBackupDatabase: v.autoBackupDatabase || '*',
    compression: v.compression ?? 6,
    retentionDays: v.retentionDays ?? 0,
    maxBackups: v.maxBackups ?? 0,
    autoHost: v.autoHost || '',
    autoPort: v.autoPort || 5432,
    autoDatabase: v.autoDatabase || 'postgres',
    autoUsername: v.autoUsername || '',
    autoSslMode: v.autoSslMode || 'disable',
    s3Enabled: v.s3Enabled,
    s3Endpoint: v.s3Endpoint || '',
    s3Region: v.s3Region || 'auto',
    s3Bucket: v.s3Bucket || '',
    s3Prefix: v.s3Prefix || '',
    s3AccessKeyId: v.s3AccessKeyId || '',
    s3ForcePathStyle: v.s3ForcePathStyle,
    s3AutoUpload: v.s3AutoUpload,
  };
}

function canRestoreFile(filename: string, engine: ServerOverview['engine']): boolean {
  if (engine === 'postgres') {
    return filename.endsWith('.dump') || filename.endsWith('.sql');
  }
  if (engine === 'mysql') {
    return filename.endsWith('.sql');
  }
  return false;
}

export function BackupsPage({
  overview,
  databases,
  onRefreshCluster,
  onNotify,
}: BackupsPageProps) {
  const [tab, setTab] = useState<BackupTab>('snapshots');
  const [caps, setCaps] = useState<ToolCapabilities | null>(null);
  const [files, setFiles] = useState<BackupFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [targetDB, setTargetDB] = useState(databases[0]?.name || overview.currentDatabase);
  const [compression, setCompression] = useState(6);
  const [runningBackup, setRunningBackup] = useState(false);

  // Upload state
  const uploadInputRef = useRef<HTMLInputElement | null>(null);
  const [uploading, setUploading] = useState(false);

  // Restore modal state
  const [restoreFile, setRestoreFile] = useState<BackupFile | null>(null);
  const [restoreTargetDB, setRestoreTargetDB] = useState('');
  const [restoreJobs, setRestoreJobs] = useState(4);
  const [cleanFirst, setCleanFirst] = useState(false);
  const [createIfMissing, setCreateIfMissing] = useState(true);
  const [restoring, setRestoring] = useState(false);

  // Delete snapshot confirmation
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [deletingFile, setDeletingFile] = useState(false);

  // Settings (Schedule + Retention + S3)
  const [settingsView, setSettingsView] = useState<BackupSettingsView | null>(null);
  const [settingsForm, setSettingsForm] = useState<BackupSettingsUpdateReq | null>(null);
  const [savingSettings, setSavingSettings] = useState(false);
  const [runningSchedule, setRunningSchedule] = useState(false);
  const [pruning, setPruning] = useState(false);

  // S3 operations state
  const [s3Objects, setS3Objects] = useState<S3Object[]>([]);
  const [loadingS3, setLoadingS3] = useState(false);
  const [testingS3, setTestingS3] = useState(false);
  const [pushingFile, setPushingFile] = useState('');
  const [pullingKey, setPullingKey] = useState('');

  const loadBackups = useCallback(async () => {
    setLoading(true);
    try {
      const [listRes, settingsRes] = await Promise.all([
        api.backups.list(),
        api.backups.getSettings(),
      ]);
      setCaps(listRes.capabilities);
      setFiles(listRes.files);
      setSettingsView(settingsRes.settings);
      setSettingsForm(toUpdateReq(settingsRes.settings));
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to load backups');
    } finally {
      setLoading(false);
    }
  }, [onNotify]);

  useEffect(() => {
    void loadBackups();
  }, [loadBackups]);

  useEffect(() => {
    if (!targetDB && databases.length > 0) {
      setTargetDB(databases[0].name);
    }
  }, [databases, targetDB]);

  const handleRefreshS3Objects = useCallback(async () => {
    setLoadingS3(true);
    try {
      const res = await api.backups.listS3();
      setS3Objects(res.objects);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to list remote S3 backups');
    } finally {
      setLoadingS3(false);
    }
  }, [onNotify]);

  const handleOpenTab = (next: BackupTab) => {
    setTab(next);
    if (next === 's3' && caps?.s3Configured && s3Objects.length === 0) {
      void handleRefreshS3Objects();
    }
  };

  const handleRunBackup = async (all: boolean) => {
    setRunningBackup(true);
    try {
      const res = await api.backups.create({
        database: targetDB,
        compression,
        all,
      });
      if (all && res.results) {
        onNotify('success', `Created ${res.results.length} snapshot backups`);
      } else if (res.result) {
        onNotify(
          'success',
          `Backup ${res.result.file.filename} (${res.result.file.sizePretty}) completed in ${res.result.durationMs}ms`
        );
      }
      await loadBackups();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Backup failed');
    } finally {
      setRunningBackup(false);
    }
  };

  const handleUploadChange = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    setUploading(true);
    try {
      const res = await api.backups.upload(file);
      onNotify('success', `Uploaded ${res.file.filename} (${res.file.sizePretty})`);
      await loadBackups();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploading(false);
    }
  };

  const openRestore = (f: BackupFile) => {
    setRestoreFile(f);
    const defaultDB =
      f.database && f.database !== 'unknown'
        ? f.database
        : targetDB || overview.currentDatabase || '';
    setRestoreTargetDB(defaultDB);
    setRestoreJobs(4);
    setCleanFirst(false);
    setCreateIfMissing(true);
  };

  const handleRestore = async (e: FormEvent) => {
    e.preventDefault();
    if (!restoreFile || !restoreTargetDB.trim()) return;
    setRestoring(true);
    try {
      const res = await api.backups.restore({
        filename: restoreFile.filename,
        targetDatabase: restoreTargetDB.trim(),
        jobs: restoreJobs,
        cleanFirst,
        createIfMissing,
      });
      onNotify('success', `Restored into "${res.database}" in ${res.durationMs}ms`);
      setRestoreFile(null);
      onRefreshCluster();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Restore failed');
    } finally {
      setRestoring(false);
    }
  };

  const handleDeleteFile = async () => {
    if (!deleteTarget) return;
    setDeletingFile(true);
    try {
      await api.backups.delete(deleteTarget);
      onNotify('success', `Deleted ${deleteTarget}`);
      setDeleteTarget(null);
      await loadBackups();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Delete failed');
    } finally {
      setDeletingFile(false);
    }
  };

  const patchForm = (patch: Partial<BackupSettingsUpdateReq>) => {
    setSettingsForm((prev) => (prev ? { ...prev, ...patch } : prev));
  };

  const handleSaveSettings = async () => {
    if (!settingsForm) return;
    setSavingSettings(true);
    try {
      const res = await api.backups.updateSettings(settingsForm);
      setSettingsView(res.settings);
      setSettingsForm(toUpdateReq(res.settings));
      onNotify('success', 'Backup settings saved');
      const listRes = await api.backups.list();
      setCaps(listRes.capabilities);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to save settings');
    } finally {
      setSavingSettings(false);
    }
  };

  const handleRunScheduleNow = async () => {
    setRunningSchedule(true);
    try {
      const res = await api.backups.runScheduleNow();
      setSettingsView(res.settings);
      setSettingsForm(toUpdateReq(res.settings));
      onNotify('success', `Scheduled run finished: ${res.results.length} snapshot(s) created`);
      await loadBackups();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Scheduled backup failed');
    } finally {
      setRunningSchedule(false);
    }
  };

  const handlePruneNow = async () => {
    setPruning(true);
    try {
      const res = await api.backups.pruneNow();
      onNotify(
        'info',
        res.deleted > 0
          ? `Pruned ${res.deleted} old backup snapshot(s)`
          : 'No old snapshots matched retention limits'
      );
      await loadBackups();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Prune failed');
    } finally {
      setPruning(false);
    }
  };

  const handleTestS3 = async () => {
    setTestingS3(true);
    try {
      await api.backups.testS3();
      onNotify('success', 'S3 connection verified successfully');
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'S3 connection test failed');
    } finally {
      setTestingS3(false);
    }
  };

  const handlePushToS3 = async (filename: string) => {
    setPushingFile(filename);
    try {
      const res = await api.backups.pushS3(filename);
      onNotify('success', `Uploaded ${filename} -> s3://${res.object.key}`);
      if (tab === 's3' || s3Objects.length > 0) {
        await handleRefreshS3Objects();
      }
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Push to S3 failed');
    } finally {
      setPushingFile('');
    }
  };

  const handlePullFromS3 = async (key: string) => {
    setPullingKey(key);
    try {
      const res = await api.backups.pullS3(key);
      onNotify('success', `Pulled ${res.file.filename} (${res.file.sizePretty}) to local storage`);
      await loadBackups();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Pull from S3 failed');
    } finally {
      setPullingKey('');
    }
  };

  const handleDeleteRemoteS3 = async (key: string) => {
    try {
      await api.backups.deleteS3(key);
      onNotify('success', `Deleted remote object ${key}`);
      setS3Objects((prev) => prev.filter((o) => o.key !== key));
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to delete remote object');
    }
  };

  const isDumpRestore = Boolean(restoreFile?.filename.endsWith('.dump'));
  const restoreMissingBinaryWarning = (() => {
    if (!restoreFile || !caps) return '';
    if (overview.engine === 'postgres') {
      if (isDumpRestore && !caps.pgRestoreAvailable) {
        return 'pg_restore binary is not available on this host.';
      }
      if (restoreFile.filename.endsWith('.sql') && !caps.psqlAvailable) {
        return 'psql client binary is not available on this host for .sql restore.';
      }
    }
    if (overview.engine === 'mysql' && !caps.mysqlClientAvailable) {
      return 'mysql client binary is not available on this host.';
    }
    return '';
  })();

  const engineBannerReady =
    overview.engine === 'sqlite' ||
    (overview.engine === 'postgres' && Boolean(caps?.pgDumpAvailable)) ||
    (overview.engine === 'mysql' && Boolean(caps?.mysqlDumpAvailable));

  return (
    <div className="space-y-6">
      {/* Hidden upload input */}
      <input
        ref={uploadInputRef}
        type="file"
        accept=".dump,.sql,.sqlite.bak"
        onChange={handleUploadChange}
        className="hidden"
      />

      {/* Header + Capability Banner */}
      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 p-5 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-500/15 text-emerald-600 dark:text-emerald-400">
                <ShieldCheck className="w-3 h-3" /> MVCC Snapshot Isolation (Zero Locking)
              </span>
              {settingsView?.autoBackupEnabled && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-indigo-500/15 text-indigo-600 dark:text-indigo-300">
                  <CalendarClock className="w-3 h-3" /> Daily @ {settingsView.autoBackupTime}
                </span>
              )}
              {caps?.s3Configured && (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-semibold bg-sky-500/15 text-sky-600 dark:text-sky-300">
                  <Cloud className="w-3 h-3" /> S3 Sync Enabled
                </span>
              )}
            </div>
            <h1 className="text-lg font-bold tracking-tight mt-1">
              Zero-Conflict Backup &amp; Restore Center
            </h1>
            <p className="text-xs text-slate-500 dark:text-slate-400">
              {overview.engine === 'postgres'
                ? 'Takes compressed .dump snapshots via pg_dump -Fc, supports .dump/.sql upload & restore (pg_restore -j / psql), automated schedules, and S3 sync.'
                : overview.engine === 'mysql'
                  ? 'Takes consistent single-transaction .sql backups via mysqldump, supports .sql upload/restore, retention pruning, and S3 sync.'
                  : 'Creates consistent online SQLite backups via VACUUM INTO, with retention pruning and S3 sync.'}
            </p>
          </div>
          <button
            type="button"
            onClick={() => void loadBackups()}
            className="self-start inline-flex items-center gap-1.5 px-3 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs font-medium hover:bg-slate-100 dark:hover:bg-slate-800"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </button>
        </div>

        {caps && (
          <div
            className={`rounded-xl border px-4 py-3 text-xs flex flex-wrap items-center justify-between gap-2 ${
              engineBannerReady
                ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                : 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300'
            }`}
          >
            <div className="flex flex-wrap items-center gap-2">
              {engineBannerReady ? (
                <CheckCircle2 className="w-4 h-4 shrink-0" />
              ) : (
                <AlertTriangle className="w-4 h-4 shrink-0" />
              )}
              {overview.engine === 'postgres' && (
                <span>
                  {caps.pgDumpAvailable
                    ? `Engine Ready: ${caps.pgDumpVersion} | pg_restore: ${caps.pgRestoreAvailable ? 'ready' : 'missing'} | psql: ${caps.psqlAvailable ? 'ready' : 'missing'}`
                    : 'pg_dump binary not installed on this host. Install postgresql-client locally or deploy on the database host.'}
                </span>
              )}
              {overview.engine === 'mysql' && (
                <span>
                  {caps.mysqlDumpAvailable
                    ? `MySQL Backup Ready | mysqldump: ready | mysql client: ${caps.mysqlClientAvailable ? 'ready' : 'missing'}`
                    : 'mysqldump binary not installed on this host.'}
                </span>
              )}
              {overview.engine === 'sqlite' && (
                <span>SQLite online snapshot backup engine ready (VACUUM INTO).</span>
              )}
            </div>
            <span className="font-mono text-[11px] opacity-80">Dir: {caps.backupDir}</span>
          </div>
        )}

        {/* Create Backup Controls */}
        <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950/60 p-4 flex flex-wrap items-end gap-3">
          <div className="flex-1 min-w-[200px]">
            <label htmlFor="manual-target-db" className="block text-[11px] font-medium text-slate-500 mb-1">
              Target Database
            </label>
            <select
              id="manual-target-db"
              value={targetDB}
              onChange={(e) => setTargetDB(e.target.value)}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-xs font-mono"
            >
              {databases
                .filter((d) => d.allowConnect)
                .map((d) => (
                  <option key={d.name} value={d.name}>
                    {d.name} ({d.sizePretty})
                  </option>
                ))}
            </select>
          </div>

          {overview.engine === 'postgres' && (
            <div className="w-44">
              <label htmlFor="manual-compression" className="block text-[11px] font-medium text-slate-500 mb-1">
                Compression (-Z 0..9)
              </label>
              <select
                id="manual-compression"
                value={compression}
                onChange={(e) => setCompression(Number(e.target.value))}
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-3 py-2 text-xs font-mono"
              >
                <option value={6}>Level 6 (Recommended)</option>
                <option value={9}>Level 9 (Smallest file)</option>
                <option value={3}>Level 3 (Fastest)</option>
                <option value={0}>Level 0 (Uncompressed)</option>
              </select>
            </div>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              disabled={runningBackup || !targetDB}
              onClick={() => void handleRunBackup(false)}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold shadow-sm"
            >
              {runningBackup ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
              Backup Selected
            </button>

            {overview.capabilities.multiDatabase && (
              <button
                type="button"
                disabled={runningBackup || databases.length === 0}
                onClick={() => void handleRunBackup(true)}
                className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-xl border border-emerald-600/40 bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 text-xs font-semibold"
              >
                <Archive className="w-3.5 h-3.5" />
                Backup All ({databases.filter((d) => d.allowConnect).length})
              </button>
            )}
          </div>
        </div>

        {/* Section Tabs */}
        <div className="flex flex-wrap items-center gap-1.5 pt-1 border-t border-slate-100 dark:border-slate-800">
          <button
            type="button"
            onClick={() => handleOpenTab('snapshots')}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium ${
              tab === 'snapshots'
                ? 'bg-indigo-600 text-white'
                : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
            }`}
          >
            <Archive className="w-3.5 h-3.5" /> Snapshots &amp; Restore ({files.length})
          </button>
          <button
            type="button"
            onClick={() => handleOpenTab('schedule')}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium ${
              tab === 'schedule'
                ? 'bg-indigo-600 text-white'
                : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
            }`}
          >
            <CalendarClock className="w-3.5 h-3.5" /> Schedule &amp; Retention
          </button>
          <button
            type="button"
            onClick={() => handleOpenTab('s3')}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium ${
              tab === 's3'
                ? 'bg-indigo-600 text-white'
                : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
            }`}
          >
            <Cloud className="w-3.5 h-3.5" /> S3 / R2 Off-Site Sync
          </button>
        </div>
      </div>

      {/* Tab Content */}
      {tab === 'snapshots' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden shadow-sm">
          <div className="px-5 py-3 border-b border-slate-200 dark:border-slate-800 flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-sm font-semibold">Saved Backup Snapshots ({files.length})</h2>
            <div className="flex items-center gap-2">
              <button
                type="button"
                disabled={uploading}
                onClick={() => uploadInputRef.current?.click()}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium disabled:opacity-50"
              >
                {uploading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Upload className="w-3.5 h-3.5" />}
                Upload Backup (.dump / .sql)
              </button>
            </div>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-900/90 text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-800">
                <tr>
                  <th className="px-4 py-3 font-semibold">Filename</th>
                  <th className="px-4 py-3 font-semibold">Database</th>
                  <th className="px-4 py-3 font-semibold">Format</th>
                  <th className="px-4 py-3 font-semibold">Size</th>
                  <th className="px-4 py-3 font-semibold">Created</th>
                  <th className="px-4 py-3 font-semibold text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70">
                {files.map((f) => (
                  <tr key={f.filename} className="hover:bg-slate-50 dark:hover:bg-slate-800/35">
                    <td className="px-4 py-3 font-mono font-semibold text-slate-900 dark:text-white">
                      {f.filename}
                    </td>
                    <td className="px-4 py-3 font-mono text-indigo-500">{f.database}</td>
                    <td className="px-4 py-3">
                      <span className="px-2 py-0.5 rounded-full bg-slate-100 dark:bg-slate-800 font-mono text-[11px]">
                        {f.format}
                      </span>
                    </td>
                    <td className="px-4 py-3 font-mono font-medium">{f.sizePretty}</td>
                    <td className="px-4 py-3 text-slate-500 font-mono">
                      {new Date(f.createdAt).toLocaleString()}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1.5">
                        <a
                          href={api.backups.downloadUrl(f.filename)}
                          className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-[11px]"
                        >
                          <Download className="w-3 h-3" /> Download
                        </a>
                        {caps?.s3Configured && (
                          <button
                            type="button"
                            disabled={pushingFile === f.filename}
                            onClick={() => void handlePushToS3(f.filename)}
                            className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-sky-600/15 hover:bg-sky-600/25 text-sky-700 dark:text-sky-300 text-[11px] font-semibold disabled:opacity-50"
                          >
                            {pushingFile === f.filename ? (
                              <Loader2 className="w-3 h-3 animate-spin" />
                            ) : (
                              <CloudUpload className="w-3 h-3" />
                            )}
                            Push S3
                          </button>
                        )}
                        {canRestoreFile(f.filename, overview.engine) && (
                          <button
                            type="button"
                            onClick={() => openRestore(f)}
                            className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-amber-600/15 hover:bg-amber-600/25 text-amber-700 dark:text-amber-300 text-[11px] font-semibold"
                          >
                            <RotateCcw className="w-3 h-3" /> Restore
                          </button>
                        )}
                        <button
                          type="button"
                          onClick={() => setDeleteTarget(f.filename)}
                          className="p-1.5 rounded-lg text-red-500 hover:bg-red-500/10"
                          aria-label={`Delete ${f.filename}`}
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
                {!loading && files.length === 0 && (
                  <tr>
                    <td colSpan={6} className="px-4 py-10 text-center text-slate-500">
                      No backup snapshots saved yet. Create a snapshot or upload a <code className="font-mono">.dump</code> / <code className="font-mono">.sql</code> file.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {tab === 'schedule' && settingsView && settingsForm && (
        <BackupSchedulePanel
          view={settingsView}
          form={settingsForm}
          engine={overview.engine}
          databases={databases}
          saving={savingSettings}
          runningNow={runningSchedule}
          pruning={pruning}
          onChange={patchForm}
          onSave={handleSaveSettings}
          onRunNow={handleRunScheduleNow}
          onPruneNow={handlePruneNow}
        />
      )}

      {tab === 's3' && settingsView && settingsForm && (
        <BackupS3Panel
          view={settingsView}
          form={settingsForm}
          objects={s3Objects}
          loadingObjects={loadingS3}
          saving={savingSettings}
          testing={testingS3}
          pullingKey={pullingKey}
          onChange={patchForm}
          onSave={handleSaveSettings}
          onTest={handleTestS3}
          onRefreshObjects={handleRefreshS3Objects}
          onPull={handlePullFromS3}
          onDeleteRemote={handleDeleteRemoteS3}
        />
      )}

      {/* Restore Modal */}
      <Modal
        open={Boolean(restoreFile)}
        onClose={() => setRestoreFile(null)}
        title={`Restore Snapshot — ${restoreFile?.filename ?? ''}`}
        subtitle={
          isDumpRestore
            ? 'Runs pg_restore -j for multi-core fast recovery into a target database.'
            : overview.engine === 'mysql'
              ? 'Imports SQL statements via the mysql CLI client into a target database.'
              : 'Executes the plain SQL backup via psql -f into a target database.'
        }
      >
        <form onSubmit={handleRestore} className="space-y-4">
          {restoreMissingBinaryWarning && (
            <div className="rounded-xl border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300 flex items-center gap-2">
              <AlertTriangle className="w-4 h-4 shrink-0" />
              <span>{restoreMissingBinaryWarning}</span>
            </div>
          )}

          <div>
            <label htmlFor="restore-target-db" className="block text-xs font-medium mb-1">
              Target Database Name (tip: restore into a new name like{' '}
              <code>{restoreFile?.database && restoreFile.database !== 'unknown' ? restoreFile.database : 'app'}_restored</code> to verify safely first!)
            </label>
            <input
              id="restore-target-db"
              type="text"
              required
              value={restoreTargetDB}
              onChange={(e) => setRestoreTargetDB(e.target.value)}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm font-mono"
            />
          </div>

          {isDumpRestore && (
            <div>
              <label htmlFor="restore-jobs" className="block text-xs font-medium mb-1">
                Parallel CPU Jobs (<code>-j</code>)
              </label>
              <select
                id="restore-jobs"
                value={restoreJobs}
                onChange={(e) => setRestoreJobs(Number(e.target.value))}
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
              >
                <option value={1}>1 job</option>
                <option value={2}>2 parallel jobs</option>
                <option value={4}>4 parallel jobs (Recommended)</option>
                <option value={8}>8 parallel jobs</option>
              </select>
            </div>
          )}

          <div className="space-y-2 text-xs">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={createIfMissing}
                onChange={(e) => setCreateIfMissing(e.target.checked)}
              />
              Auto-create target database if it does not exist
            </label>
            {isDumpRestore && (
              <label className="flex items-center gap-2 cursor-pointer text-amber-600 dark:text-amber-300">
                <input
                  type="checkbox"
                  checked={cleanFirst}
                  onChange={(e) => setCleanFirst(e.target.checked)}
                />
                Drop &amp; recreate existing objects (<code>--clean --if-exists</code>)
              </label>
            )}
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setRestoreFile(null)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={restoring}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-amber-600 hover:bg-amber-500 disabled:opacity-50 text-white text-xs font-semibold"
            >
              {restoring && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              {isDumpRestore ? 'Run Parallel Restore' : 'Restore SQL Backup'}
            </button>
          </div>
        </form>
      </Modal>

      <ConfirmModal
        open={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        title="Delete Backup Snapshot"
        confirmLabel="Delete Snapshot"
        loading={deletingFile}
        onConfirm={handleDeleteFile}
        message={
          <>
            Permanently remove backup file{' '}
            <code className="font-mono font-semibold">{deleteTarget}</code> from disk?
          </>
        }
      />
    </div>
  );
}

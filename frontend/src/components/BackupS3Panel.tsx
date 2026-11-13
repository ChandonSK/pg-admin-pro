import { useState, type FormEvent } from 'react';
import {
  Cloud,
  CloudDownload,
  Loader2,
  PlugZap,
  RefreshCw,
  Save,
  Trash2,
} from 'lucide-react';
import type {
  BackupSettingsUpdateReq,
  BackupSettingsView,
  S3Object,
} from '../types';
import { ConfirmModal } from './ConfirmModal';

interface BackupS3PanelProps {
  view: BackupSettingsView;
  form: BackupSettingsUpdateReq;
  objects: S3Object[];
  loadingObjects: boolean;
  saving: boolean;
  testing: boolean;
  pullingKey: string;
  onChange: (patch: Partial<BackupSettingsUpdateReq>) => void;
  onSave: () => Promise<void>;
  onTest: () => Promise<void>;
  onRefreshObjects: () => Promise<void>;
  onPull: (key: string) => Promise<void>;
  onDeleteRemote: (key: string) => Promise<void>;
}

export function BackupS3Panel({
  view,
  form,
  objects,
  loadingObjects,
  saving,
  testing,
  pullingKey,
  onChange,
  onSave,
  onTest,
  onRefreshObjects,
  onPull,
  onDeleteRemote,
}: BackupS3PanelProps) {
  const [deleteKey, setDeleteKey] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    await onSave();
  };

  const confirmDeleteRemote = async () => {
    if (!deleteKey) return;
    setDeleting(true);
    try {
      await onDeleteRemote(deleteKey);
      setDeleteKey(null);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="space-y-5">
      <form
        onSubmit={handleSubmit}
        className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 p-5 space-y-5 shadow-sm"
      >
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-bold flex items-center gap-2">
              <Cloud className="w-4 h-4 text-sky-500" />
              S3 / Cloudflare R2 Off-Site Backup Sync
            </h2>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
              Zero-dependency AWS SigV4 sync compatible with Cloudflare R2, MinIO, AWS S3, and Backblaze B2.
            </p>
          </div>
          <button
            type="button"
            disabled={testing}
            onClick={() => void onTest()}
            className="self-start inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-sky-500/40 bg-sky-500/10 hover:bg-sky-500/20 text-sky-700 dark:text-sky-300 text-xs font-semibold disabled:opacity-50"
          >
            {testing ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <PlugZap className="w-3.5 h-3.5" />}
            Test Saved S3 Connection
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="flex items-center justify-between rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950/60 px-3.5 py-2.5">
            <div>
              <div className="text-xs font-semibold">Enable S3 / R2 Integration</div>
              <div className="text-[11px] text-slate-500">Allows pushing &amp; pulling snapshots</div>
            </div>
            <input
              id="s3-enabled"
              type="checkbox"
              checked={form.s3Enabled}
              onChange={(e) => onChange({ s3Enabled: e.target.checked })}
              className="h-4 w-4 accent-indigo-600 cursor-pointer"
            />
          </div>

          <div className="flex items-center justify-between rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950/60 px-3.5 py-2.5">
            <div>
              <div className="text-xs font-semibold">Auto-Upload New Backups</div>
              <div className="text-[11px] text-slate-500">Uploads right after backup completes</div>
            </div>
            <input
              id="s3-autoupload"
              type="checkbox"
              checked={form.s3AutoUpload}
              onChange={(e) => onChange({ s3AutoUpload: e.target.checked })}
              className="h-4 w-4 accent-indigo-600 cursor-pointer"
            />
          </div>

          <div className="flex items-center justify-between rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950/60 px-3.5 py-2.5">
            <div>
              <div className="text-xs font-semibold">Path-Style URLs</div>
              <div className="text-[11px] text-slate-500">Recommended for R2 / MinIO</div>
            </div>
            <input
              id="s3-pathstyle"
              type="checkbox"
              checked={form.s3ForcePathStyle}
              onChange={(e) => onChange({ s3ForcePathStyle: e.target.checked })}
              className="h-4 w-4 accent-indigo-600 cursor-pointer"
            />
          </div>

          <div>
            <label htmlFor="s3-endpoint" className="block text-[11px] font-medium text-slate-500 mb-1">
              Endpoint URL (blank for standard AWS S3)
            </label>
            <input
              id="s3-endpoint"
              type="url"
              value={form.s3Endpoint}
              onChange={(e) => onChange({ s3Endpoint: e.target.value })}
              placeholder="https://<account>.r2.cloudflarestorage.com"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="s3-region" className="block text-[11px] font-medium text-slate-500 mb-1">
              Region
            </label>
            <input
              id="s3-region"
              type="text"
              value={form.s3Region}
              onChange={(e) => onChange({ s3Region: e.target.value })}
              placeholder="auto or us-east-1"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="s3-bucket" className="block text-[11px] font-medium text-slate-500 mb-1">
              Bucket Name
            </label>
            <input
              id="s3-bucket"
              type="text"
              value={form.s3Bucket}
              onChange={(e) => onChange({ s3Bucket: e.target.value })}
              placeholder="db-backups"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="s3-prefix" className="block text-[11px] font-medium text-slate-500 mb-1">
              Key Prefix
            </label>
            <input
              id="s3-prefix"
              type="text"
              value={form.s3Prefix}
              onChange={(e) => onChange({ s3Prefix: e.target.value })}
              placeholder="pg-admin-pro"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="s3-access-key" className="block text-[11px] font-medium text-slate-500 mb-1">
              Access Key ID
            </label>
            <input
              id="s3-access-key"
              type="text"
              value={form.s3AccessKeyId}
              onChange={(e) => onChange({ s3AccessKeyId: e.target.value })}
              placeholder="AKIA..."
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
          </div>

          <div>
            <label htmlFor="s3-secret-key" className="block text-[11px] font-medium text-slate-500 mb-1">
              Secret Access Key {view.hasS3SecretKey && <span className="text-emerald-500">(saved encrypted)</span>}
            </label>
            <input
              id="s3-secret-key"
              type="password"
              value={form.s3SecretKey || ''}
              onChange={(e) => onChange({ s3SecretKey: e.target.value, clearS3SecretKey: false })}
              placeholder={view.hasS3SecretKey ? 'Leave blank to keep existing' : 'Enter secret access key'}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono"
            />
            {view.hasS3SecretKey && (
              <label className="mt-1 inline-flex items-center gap-1.5 text-[11px] text-slate-500 cursor-pointer">
                <input
                  type="checkbox"
                  checked={Boolean(form.clearS3SecretKey)}
                  onChange={(e) =>
                    onChange({
                      clearS3SecretKey: e.target.checked,
                      s3SecretKey: e.target.checked ? '' : form.s3SecretKey,
                    })
                  }
                />
                Clear saved secret key
              </label>
            )}
          </div>
        </div>

        <div className="flex justify-end">
          <button
            type="submit"
            disabled={saving}
            className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold shadow-sm"
          >
            {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Save className="w-3.5 h-3.5" />}
            Save S3 Settings
          </button>
        </div>
      </form>

      {/* Remote S3 Object Browser */}
      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden shadow-sm">
        <div className="px-5 py-3 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">Remote Bucket Backups ({objects.length})</h3>
          <button
            type="button"
            disabled={loadingObjects}
            onClick={() => void onRefreshObjects()}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium disabled:opacity-50"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loadingObjects ? 'animate-spin' : ''}`} />
            List Remote Objects
          </button>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-slate-50 dark:bg-slate-900/90 text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-800">
              <tr>
                <th className="px-4 py-3 font-semibold">Filename</th>
                <th className="px-4 py-3 font-semibold">Object Key</th>
                <th className="px-4 py-3 font-semibold">Size</th>
                <th className="px-4 py-3 font-semibold">Last Modified</th>
                <th className="px-4 py-3 font-semibold text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70">
              {objects.map((obj) => (
                <tr key={obj.key} className="hover:bg-slate-50 dark:hover:bg-slate-800/35">
                  <td className="px-4 py-3 font-mono font-semibold text-slate-900 dark:text-white">
                    {obj.filename}
                  </td>
                  <td className="px-4 py-3 font-mono text-[11px] text-slate-500 max-w-xs truncate">
                    {obj.key}
                  </td>
                  <td className="px-4 py-3 font-mono font-medium">{obj.sizePretty}</td>
                  <td className="px-4 py-3 font-mono text-slate-500">
                    {obj.lastModified ? new Date(obj.lastModified).toLocaleString() : '-'}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-1.5">
                      <button
                        type="button"
                        disabled={pullingKey === obj.key}
                        onClick={() => void onPull(obj.key)}
                        className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-sky-600/15 hover:bg-sky-600/25 text-sky-700 dark:text-sky-300 text-[11px] font-semibold disabled:opacity-50"
                      >
                        {pullingKey === obj.key ? (
                          <Loader2 className="w-3 h-3 animate-spin" />
                        ) : (
                          <CloudDownload className="w-3 h-3" />
                        )}
                        Pull to Local
                      </button>
                      <button
                        type="button"
                        onClick={() => setDeleteKey(obj.key)}
                        className="p-1.5 rounded-lg text-red-500 hover:bg-red-500/10"
                        aria-label={`Delete remote ${obj.filename}`}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {!loadingObjects && objects.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500">
                    {view.s3Enabled
                      ? 'No remote backup objects listed yet. Click "List Remote Objects" or push a snapshot.'
                      : 'Configure and enable S3/R2 above to browse off-site snapshots.'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <ConfirmModal
        open={Boolean(deleteKey)}
        onClose={() => setDeleteKey(null)}
        title="Delete Remote S3 Backup"
        confirmLabel="Delete Remote Object"
        loading={deleting}
        onConfirm={confirmDeleteRemote}
        message={
          <>
            Permanently delete remote object{' '}
            <code className="font-mono font-semibold break-all">{deleteKey}</code> from S3/R2?
          </>
        }
      />
    </div>
  );
}

import { useCallback, useEffect, useState } from 'react';
import {
  Blocks,
  Code2,
  Cpu,
  Hash,
  Loader2,
  Plus,
  RefreshCw,
  Shapes,
  Trash2,
  Zap,
} from 'lucide-react';
import { api } from '../api/client';
import type { DatabaseObjects, DBExtensionInfo } from '../types';
import { ConfirmModal } from './ConfirmModal';
import { Modal } from './Modal';
import type { ToastItem } from './Toast';

interface DatabaseObjectsPanelProps {
  dbName: string;
  schema: string;
  onNotify: (kind: ToastItem['kind'], msg: string) => void;
}

type ObjTab = 'functions' | 'triggers' | 'sequences' | 'types' | 'extensions';

export function DatabaseObjectsPanel({
  dbName,
  schema,
  onNotify,
}: DatabaseObjectsPanelProps) {
  const [objects, setObjects] = useState<DatabaseObjects | null>(null);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<ObjTab>('functions');
  const [defModal, setDefModal] = useState<{ title: string; subtitle: string; sql: string } | null>(
    null
  );
  const [extBusy, setExtBusy] = useState('');
  const [dropExt, setDropExt] = useState<DBExtensionInfo | null>(null);

  const loadObjects = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.tables.objects(dbName, schema);
      setObjects(res);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to load database objects');
    } finally {
      setLoading(false);
    }
  }, [dbName, schema, onNotify]);

  useEffect(() => {
    void loadObjects();
  }, [loadObjects]);

  const handleInstallExtension = async (ext: DBExtensionInfo) => {
    setExtBusy(ext.name);
    try {
      await api.tables.manageExtension(dbName, ext.name, 'install', schema);
      onNotify('success', `Extension "${ext.name}" installed`);
      await loadObjects();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Install extension failed');
    } finally {
      setExtBusy('');
    }
  };

  const handleDropExtension = async () => {
    if (!dropExt) return;
    setExtBusy(dropExt.name);
    try {
      await api.tables.manageExtension(dbName, dropExt.name, 'drop');
      onNotify('success', `Dropped extension "${dropExt.name}"`);
      setDropExt(null);
      await loadObjects();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Drop extension failed');
    } finally {
      setExtBusy('');
    }
  };

  if (loading && !objects) {
    return (
      <div className="py-16 text-center text-xs text-slate-500">
        <Loader2 className="w-5 h-5 animate-spin mx-auto mb-2 text-indigo-500" />
        Loading objects in {dbName} ({schema})...
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-1.5">
          {[
            { id: 'functions' as const, label: `Functions (${objects?.functions.length ?? 0})`, icon: Cpu },
            { id: 'triggers' as const, label: `Triggers (${objects?.triggers.length ?? 0})`, icon: Zap },
            { id: 'sequences' as const, label: `Sequences (${objects?.sequences.length ?? 0})`, icon: Hash },
            { id: 'types' as const, label: `Types & ENUMs (${objects?.types.length ?? 0})`, icon: Shapes },
            { id: 'extensions' as const, label: `Extensions (${objects?.extensions.length ?? 0})`, icon: Blocks },
          ].map((item) => {
            const Icon = item.icon;
            const active = tab === item.id;
            return (
              <button
                key={item.id}
                type="button"
                onClick={() => setTab(item.id)}
                className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-medium transition-colors ${
                  active
                    ? 'bg-indigo-600 text-white'
                    : 'bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'
                }`}
              >
                <Icon className="w-3.5 h-3.5" />
                {item.label}
              </button>
            );
          })}
        </div>

        <button
          type="button"
          onClick={() => void loadObjects()}
          disabled={loading}
          className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 text-xs hover:bg-slate-100 dark:hover:bg-slate-800"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      {/* Functions */}
      {tab === 'functions' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 text-[11px] uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-2.5">Name</th>
                  <th className="px-4 py-2.5">Arguments</th>
                  <th className="px-4 py-2.5">Returns</th>
                  <th className="px-4 py-2.5">Language / Kind</th>
                  <th className="px-4 py-2.5 text-right">Definition</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {(objects?.functions || []).length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-8 text-center text-slate-400">
                      No stored functions or procedures in schema &ldquo;{schema}&rdquo;.
                    </td>
                  </tr>
                ) : (
                  (objects?.functions || []).map((fn, i) => (
                    <tr key={`${fn.name}-${i}`}>
                      <td className="px-4 py-2.5 font-mono font-semibold text-indigo-500">
                        {fn.schema}.{fn.name}
                      </td>
                      <td className="px-4 py-2.5 font-mono text-[11px] max-w-xs truncate">
                        ({fn.args || 'none'})
                      </td>
                      <td className="px-4 py-2.5 font-mono text-[11px]">{fn.returnType}</td>
                      <td className="px-4 py-2.5">
                        <span className="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-[10px] font-mono">
                          {fn.language} &middot; {fn.kind}
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-right">
                        <button
                          type="button"
                          onClick={() =>
                            setDefModal({
                              title: `${fn.schema}.${fn.name}(${fn.args})`,
                              subtitle: `${fn.language} ${fn.kind} returning ${fn.returnType}`,
                              sql: fn.definition,
                            })
                          }
                          className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-[11px]"
                        >
                          <Code2 className="w-3 h-3" />
                          View SQL
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

      {/* Triggers */}
      {tab === 'triggers' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 text-[11px] uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-2.5">Trigger Name</th>
                  <th className="px-4 py-2.5">Table</th>
                  <th className="px-4 py-2.5">Event</th>
                  <th className="px-4 py-2.5">Status</th>
                  <th className="px-4 py-2.5 text-right">Definition</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {(objects?.triggers || []).length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-8 text-center text-slate-400">
                      No triggers in schema &ldquo;{schema}&rdquo;.
                    </td>
                  </tr>
                ) : (
                  (objects?.triggers || []).map((tr, i) => (
                    <tr key={`${tr.table}-${tr.name}-${i}`}>
                      <td className="px-4 py-2.5 font-mono font-semibold">{tr.name}</td>
                      <td className="px-4 py-2.5 font-mono text-indigo-500">{tr.table}</td>
                      <td className="px-4 py-2.5 font-mono text-[11px]">{tr.event}</td>
                      <td className="px-4 py-2.5">
                        <span
                          className={`px-2 py-0.5 rounded-full text-[10px] font-semibold ${
                            tr.enabled
                              ? 'bg-emerald-500/15 text-emerald-500'
                              : 'bg-slate-500/15 text-slate-400'
                          }`}
                        >
                          {tr.enabled ? 'ENABLED' : 'DISABLED'}
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-right">
                        <button
                          type="button"
                          onClick={() =>
                            setDefModal({
                              title: tr.name,
                              subtitle: `Trigger on ${tr.table}`,
                              sql: tr.definition,
                            })
                          }
                          className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-[11px]"
                        >
                          <Code2 className="w-3 h-3" />
                          View SQL
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

      {/* Sequences */}
      {tab === 'sequences' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 text-[11px] uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-2.5">Sequence Name</th>
                  <th className="px-4 py-2.5">Type</th>
                  <th className="px-4 py-2.5">Last Value</th>
                  <th className="px-4 py-2.5">Increment</th>
                  <th className="px-4 py-2.5">Range</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {(objects?.sequences || []).length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-8 text-center text-slate-400">
                      No sequences found in schema &ldquo;{schema}&rdquo;.
                    </td>
                  </tr>
                ) : (
                  (objects?.sequences || []).map((seq) => (
                    <tr key={`${seq.schema}.${seq.name}`}>
                      <td className="px-4 py-2.5 font-mono font-semibold text-indigo-500">
                        {seq.schema}.{seq.name}
                      </td>
                      <td className="px-4 py-2.5 font-mono">{seq.dataType}</td>
                      <td className="px-4 py-2.5 font-mono font-bold">
                        {seq.lastValue !== null ? seq.lastValue.toLocaleString() : 'not yet used'}
                      </td>
                      <td className="px-4 py-2.5 font-mono">+{seq.incrementBy}</td>
                      <td className="px-4 py-2.5 font-mono text-[11px] text-slate-500">
                        {seq.minValue} .. {seq.maxValue.toLocaleString()}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Custom Types & ENUMs */}
      {tab === 'types' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 divide-y divide-slate-100 dark:divide-slate-800">
          {(objects?.types || []).length === 0 ? (
            <div className="px-4 py-8 text-center text-xs text-slate-400">
              No custom types or ENUMs in schema &ldquo;{schema}&rdquo;.
            </div>
          ) : (
            (objects?.types || []).map((t) => (
              <div key={`${t.schema}.${t.name}`} className="p-4 space-y-2">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-xs font-bold text-indigo-500">
                    {t.schema}.{t.name}
                  </span>
                  <span className="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-[10px] font-semibold">
                    {t.kind}
                  </span>
                </div>
                {t.values && t.values.length > 0 && (
                  <div className="flex flex-wrap gap-1.5">
                    {t.values.map((v) => (
                      <span
                        key={v}
                        className="px-2 py-0.5 rounded-md bg-emerald-500/10 border border-emerald-500/25 text-emerald-600 dark:text-emerald-300 text-[11px] font-mono"
                      >
                        {v}
                      </span>
                    ))}
                  </div>
                )}
                {t.definition && (
                  <p className="text-[11px] font-mono text-slate-500">{t.definition}</p>
                )}
              </div>
            ))
          )}
        </div>
      )}

      {/* Extensions */}
      {tab === 'extensions' && (
        <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800 text-[11px] uppercase text-slate-500">
                <tr>
                  <th className="px-4 py-2.5">Extension</th>
                  <th className="px-4 py-2.5">Status / Version</th>
                  <th className="px-4 py-2.5">Description</th>
                  <th className="px-4 py-2.5 text-right">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {(objects?.extensions || []).length === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-4 py-8 text-center text-slate-400">
                      Extensions catalog is available on PostgreSQL servers.
                    </td>
                  </tr>
                ) : (
                  (objects?.extensions || []).map((ext) => (
                    <tr key={ext.name}>
                      <td className="px-4 py-2.5 font-mono font-semibold">{ext.name}</td>
                      <td className="px-4 py-2.5">
                        {ext.installed ? (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-500 text-[10px] font-semibold">
                            Installed ({ext.installedVersion})
                          </span>
                        ) : (
                          <span className="text-[11px] font-mono text-slate-400">
                            Available (v{ext.defaultVersion})
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-2.5 text-slate-500 max-w-md">{ext.comment}</td>
                      <td className="px-4 py-2.5 text-right whitespace-nowrap">
                        {ext.installed ? (
                          ext.name !== 'plpgsql' && (
                            <button
                              type="button"
                              disabled={extBusy === ext.name}
                              onClick={() => setDropExt(ext)}
                              className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-red-500/30 text-red-500 text-[11px] hover:bg-red-500/10"
                            >
                              <Trash2 className="w-3 h-3" />
                              Drop
                            </button>
                          )
                        ) : (
                          <button
                            type="button"
                            disabled={extBusy === ext.name}
                            onClick={() => void handleInstallExtension(ext)}
                            className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-[11px] font-medium"
                          >
                            {extBusy === ext.name ? (
                              <Loader2 className="w-3 h-3 animate-spin" />
                            ) : (
                              <Plus className="w-3 h-3" />
                            )}
                            Install
                          </button>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Definition Modal */}
      <Modal
        open={!!defModal}
        onClose={() => setDefModal(null)}
        title={defModal?.title || 'Object SQL'}
        subtitle={defModal?.subtitle}
        maxWidthClass="max-w-3xl"
      >
        <pre className="rounded-xl bg-slate-950 text-slate-100 p-4 text-xs font-mono whitespace-pre-wrap overflow-x-auto max-h-[65vh]">
          {defModal?.sql || '(no definition returned)'}
        </pre>
      </Modal>

      <ConfirmModal
        open={!!dropExt}
        onClose={() => setDropExt(null)}
        title="Drop Extension"
        subtitle={dbName}
        confirmTextMatch={dropExt?.name}
        confirmLabel="Drop Extension"
        loading={Boolean(extBusy)}
        onConfirm={handleDropExtension}
        message={
          <>
            Drop extension <strong className="font-mono">{dropExt?.name}</strong> from database{' '}
            <strong className="font-mono">{dbName}</strong>?
          </>
        }
      />
    </div>
  );
}

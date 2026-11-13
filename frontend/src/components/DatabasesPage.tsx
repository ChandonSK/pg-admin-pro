import { useState, type FormEvent } from 'react';
import {
  Archive,
  ArrowRight,
  Database,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  UserPlus,
} from 'lucide-react';
import { api } from '../api/client';
import type { DatabaseInfo, ServerOverview, UserInfo } from '../types';
import { ConfirmModal } from './ConfirmModal';
import { DatabaseGrantsPanel } from './DatabaseGrantsPanel';
import { Modal } from './Modal';

interface DatabasesPageProps {
  overview: ServerOverview;
  databases: DatabaseInfo[];
  users: UserInfo[];
  onRefresh: () => void;
  onEnterDatabase: (dbName: string) => void;
  onNotify: (kind: 'success' | 'error' | 'info', msg: string) => void;
}

export function DatabasesPage({
  overview,
  databases,
  users,
  onRefresh,
  onEnterDatabase,
  onNotify,
}: DatabasesPageProps) {
  const [search, setSearch] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [newOwner, setNewOwner] = useState(overview.currentUser);
  const [newEncoding, setNewEncoding] = useState('UTF8');
  const [submitting, setSubmitting] = useState(false);

  const [grantsDB, setGrantsDB] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const [backingUpDB, setBackingUpDB] = useState<string | null>(null);

  const filtered = databases.filter(
    (d) =>
      d.name.toLowerCase().includes(search.toLowerCase()) ||
      d.owner.toLowerCase().includes(search.toLowerCase())
  );

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    setSubmitting(true);
    try {
      await api.databases.create({
        name: newName.trim(),
        owner: newOwner,
        encoding: newEncoding,
      });
      onNotify('success', `Database "${newName.trim()}" created`);
      setCreateOpen(false);
      setNewName('');
      onRefresh();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Create database failed');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDrop = async () => {
    if (!dropTarget) return;
    setSubmitting(true);
    try {
      await api.databases.drop(dropTarget);
      onNotify('success', `Database "${dropTarget}" dropped`);
      setDropTarget(null);
      onRefresh();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Drop database failed');
    } finally {
      setSubmitting(false);
    }
  };

  const handleQuickBackup = async (dbName: string) => {
    setBackingUpDB(dbName);
    try {
      const res = await api.backups.create({ database: dbName, compression: 6 });
      if (res.result) {
        onNotify(
          'success',
          `Backup ${res.result.file.filename} created (${res.result.file.sizePretty} in ${res.result.durationMs}ms)`
        );
      }
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Backup failed');
    } finally {
      setBackingUpDB(null);
    }
  };

  return (
    <div className="space-y-5">
      {/* Toolbar */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-bold tracking-tight">Databases</h1>
          <p className="text-xs text-slate-500 dark:text-slate-400">
            Create databases, assign users/owners, take zero-conflict backups, or enter a database to inspect tables &amp; rows.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search databases..."
              className="pl-8 pr-3 py-1.5 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>
          <button
            type="button"
            onClick={onRefresh}
            className="p-2 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800"
            aria-label="Refresh databases"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
          {overview.capabilities.multiDatabase && (
            <button
              type="button"
              onClick={() => setCreateOpen(true)}
              className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-sm"
            >
              <Plus className="w-3.5 h-3.5" />
              New Database
            </button>
          )}
        </div>
      </div>

      {/* Table */}
      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-slate-50 dark:bg-slate-900/90 text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-800">
              <tr>
                <th className="px-4 py-3 font-semibold">Database Name</th>
                <th className="px-4 py-3 font-semibold">Owner</th>
                <th className="px-4 py-3 font-semibold">Assigned Users</th>
                <th className="px-4 py-3 font-semibold">Size</th>
                <th className="px-4 py-3 font-semibold">Encoding</th>
                <th className="px-4 py-3 font-semibold">Conns</th>
                <th className="px-4 py-3 font-semibold text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
              {filtered.map((db) => (
                <tr
                  key={db.name}
                  className="hover:bg-slate-50 dark:hover:bg-slate-800/35 transition-colors"
                >
                  <td className="px-4 py-3">
                    <button
                      type="button"
                      disabled={!db.allowConnect}
                      onClick={() => onEnterDatabase(db.name)}
                      className="inline-flex items-center gap-2 font-mono font-semibold text-sm text-indigo-600 dark:text-indigo-400 hover:underline disabled:opacity-50"
                    >
                      <Database className="w-3.5 h-3.5 shrink-0" />
                      {db.name}
                    </button>
                  </td>
                  <td className="px-4 py-3 font-mono">{db.owner}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1 max-w-xs">
                      {db.assignedUsers.slice(0, 4).map((u) => (
                        <span
                          key={u}
                          className="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 font-mono text-[11px]"
                        >
                          {u}
                        </span>
                      ))}
                      {db.assignedUsers.length > 4 && (
                        <span className="text-[11px] text-slate-400">
                          +{db.assignedUsers.length - 4}
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 font-mono font-medium">{db.sizePretty}</td>
                  <td className="px-4 py-3 font-mono text-slate-500">{db.encoding}</td>
                  <td className="px-4 py-3 font-mono">{db.activeConns}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-1.5">
                      <button
                        type="button"
                        disabled={!db.allowConnect}
                        onClick={() => onEnterDatabase(db.name)}
                        className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white text-[11px] font-semibold"
                      >
                        Enter <ArrowRight className="w-3 h-3" />
                      </button>

                      {overview.capabilities.roleAssignment && (
                        <button
                          type="button"
                          onClick={() => setGrantsDB(db.name)}
                          title="Assign users to database"
                          className="inline-flex items-center gap-1 px-2 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-[11px]"
                        >
                          <UserPlus className="w-3 h-3 text-emerald-500" />
                          Users
                        </button>
                      )}

                      <button
                        type="button"
                        disabled={backingUpDB === db.name}
                        onClick={() => handleQuickBackup(db.name)}
                        title="Create binary snapshot backup"
                        className="inline-flex items-center gap-1 px-2 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-[11px]"
                      >
                        {backingUpDB === db.name ? (
                          <Loader2 className="w-3 h-3 animate-spin" />
                        ) : (
                          <Archive className="w-3 h-3 text-amber-500" />
                        )}
                        Backup
                      </button>

                      {overview.capabilities.multiDatabase &&
                        db.name !== 'postgres' &&
                        db.name !== overview.currentDatabase && (
                          <button
                            type="button"
                            onClick={() => setDropTarget(db.name)}
                            title="Drop database"
                            className="p-1.5 rounded-lg text-red-500 hover:bg-red-500/10"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        )}
                    </div>
                  </td>
                </tr>
              ))}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-slate-500">
                    No matching databases found.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create Database Modal */}
      <Modal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Create New Database"
        subtitle="Provision an isolated database (e.g. per college or service)."
      >
        <form onSubmit={handleCreate} className="space-y-4">
          <div>
            <label className="block text-xs font-medium mb-1">Database Name</label>
            <input
              type="text"
              required
              autoFocus
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="edulentra_college_02"
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          {overview.engine === 'postgres' && (
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs font-medium mb-1">Owner Role</label>
                <select
                  value={newOwner}
                  onChange={(e) => setNewOwner(e.target.value)}
                  className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
                >
                  {users.map((u) => (
                    <option key={u.name} value={u.name}>
                      {u.name}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Encoding</label>
                <select
                  value={newEncoding}
                  onChange={(e) => setNewEncoding(e.target.value)}
                  className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
                >
                  <option value="UTF8">UTF8</option>
                  <option value="SQL_ASCII">SQL_ASCII</option>
                </select>
              </div>
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setCreateOpen(false)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs font-medium"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold"
            >
              {submitting && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Create Database
            </button>
          </div>
        </form>
      </Modal>

      {/* Assign Users Modal */}
      <Modal
        open={Boolean(grantsDB)}
        onClose={() => setGrantsDB(null)}
        title={`Database Users & Access — ${grantsDB ?? ''}`}
        subtitle="Assign ownership, admin, read-write, or read-only permissions."
        maxWidthClass="max-w-2xl"
      >
        {grantsDB && (
          <DatabaseGrantsPanel
            dbName={grantsDB}
            users={users}
            supportsSchemas={overview.capabilities.schemas}
            onNotify={onNotify}
            onChanged={onRefresh}
          />
        )}
      </Modal>

      {/* Drop Database Confirmation Modal */}
      <ConfirmModal
        open={Boolean(dropTarget)}
        onClose={() => setDropTarget(null)}
        title="Drop Database"
        subtitle="This permanently deletes all tables and data in the database."
        message={
          <>
            You are about to permanently drop database{' '}
            <code className="font-mono font-semibold">{dropTarget}</code>. Active connections will
            be terminated and all data will be lost.
          </>
        }
        confirmTextMatch={dropTarget ?? undefined}
        confirmLabel="Drop Permanently"
        loading={submitting}
        onConfirm={handleDrop}
      />
    </div>
  );
}

import { useState, type FormEvent } from 'react';
import {
  Database,
  Edit3,
  KeyRound,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Shield,
  Trash2,
} from 'lucide-react';
import { api } from '../api/client';
import type { DatabaseInfo, ServerOverview, UserInfo } from '../types';
import { ConfirmModal } from './ConfirmModal';
import { Modal } from './Modal';

interface UsersPageProps {
  overview: ServerOverview;
  users: UserInfo[];
  databases: DatabaseInfo[];
  onRefresh: () => void;
  onNotify: (kind: 'success' | 'error' | 'info', msg: string) => void;
}

export function UsersPage({
  overview,
  users,
  databases,
  onRefresh,
  onNotify,
}: UsersPageProps) {
  const [search, setSearch] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<UserInfo | null>(null);
  const [assignTarget, setAssignTarget] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Create form
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [canLogin, setCanLogin] = useState(true);
  const [isSuperuser, setIsSuperuser] = useState(false);
  const [canCreateDb, setCanCreateDb] = useState(false);
  const [canCreateRole, setCanCreateRole] = useState(false);
  const [connLimit, setConnLimit] = useState(-1);

  // Edit form
  const [editPass, setEditPass] = useState('');
  const [editLogin, setEditLogin] = useState(true);
  const [editSuper, setEditSuper] = useState(false);
  const [editCreateDb, setEditCreateDb] = useState(false);
  const [editCreateRole, setEditCreateRole] = useState(false);
  const [editConnLimit, setEditConnLimit] = useState(-1);

  // Assign user to DB form
  const [assignDB, setAssignDB] = useState('');
  const [assignPreset, setAssignPreset] = useState('read_write');
  const [assignDefaults, setAssignDefaults] = useState(true);

  const filtered = users.filter((u) =>
    u.name.toLowerCase().includes(search.toLowerCase())
  );

  const handleCreateUser = async (e: FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    try {
      await api.users.create({
        username: username.trim(),
        password,
        canLogin,
        isSuperuser,
        canCreateDb,
        canCreateRole,
        connLimit: Number(connLimit),
      });
      onNotify('success', `User "${username.trim()}" created`);
      setCreateOpen(false);
      setUsername('');
      setPassword('');
      onRefresh();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Create user failed');
    } finally {
      setSubmitting(false);
    }
  };

  const openEditModal = (u: UserInfo) => {
    setEditTarget(u);
    setEditPass('');
    setEditLogin(u.canLogin);
    setEditSuper(u.isSuperuser);
    setEditCreateDb(u.canCreateDb);
    setEditCreateRole(u.canCreateRole);
    setEditConnLimit(u.connLimit);
  };

  const handleUpdateUser = async (e: FormEvent) => {
    e.preventDefault();
    if (!editTarget) return;
    setSubmitting(true);
    try {
      await api.users.update(editTarget.name, {
        password: editPass ? editPass : undefined,
        canLogin: editLogin,
        isSuperuser: editSuper,
        canCreateDb: editCreateDb,
        canCreateRole: editCreateRole,
        connLimit: Number(editConnLimit),
      });
      onNotify('success', `Updated user "${editTarget.name}"`);
      setEditTarget(null);
      onRefresh();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Update user failed');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDropUser = async () => {
    if (!dropTarget) return;
    setSubmitting(true);
    try {
      await api.users.drop(dropTarget);
      onNotify('success', `Dropped user "${dropTarget}"`);
      setDropTarget(null);
      onRefresh();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Drop user failed');
    } finally {
      setSubmitting(false);
    }
  };

  const openAssignModal = (uname: string) => {
    setAssignTarget(uname);
    setAssignDB(databases[0]?.name || '');
    setAssignPreset('read_write');
    setAssignDefaults(true);
  };

  const handleAssignToDB = async (e: FormEvent) => {
    e.preventDefault();
    if (!assignTarget || !assignDB) return;
    setSubmitting(true);
    try {
      await api.databases.assignGrant(assignDB, {
        username: assignTarget,
        preset: assignPreset,
        schema: 'public',
        applyDefaultPrivs: assignDefaults,
      });
      onNotify('success', `Assigned "${assignTarget}" (${assignPreset}) to "${assignDB}"`);
      setAssignTarget(null);
      onRefresh();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Assign user failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div>
          <h1 className="text-lg font-bold tracking-tight">Users &amp; Roles</h1>
          <p className="text-xs text-slate-500 dark:text-slate-400">
            Create database users, edit role privileges or passwords, and assign users directly to databases.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search roles..."
              className="pl-8 pr-3 py-1.5 rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>
          <button
            type="button"
            onClick={onRefresh}
            className="p-2 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800"
            aria-label="Refresh users"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            onClick={() => setCreateOpen(true)}
            className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-sm"
          >
            <Plus className="w-3.5 h-3.5" />
            Create User
          </button>
        </div>
      </div>

      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900/70 overflow-hidden shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-slate-50 dark:bg-slate-900/90 text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-800">
              <tr>
                <th className="px-4 py-3 font-semibold">Username / Role</th>
                <th className="px-4 py-3 font-semibold">Privileges</th>
                <th className="px-4 py-3 font-semibold">Conn Limit</th>
                <th className="px-4 py-3 font-semibold">Owned Databases</th>
                <th className="px-4 py-3 font-semibold text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">
              {filtered.map((u) => (
                <tr key={u.name} className="hover:bg-slate-50 dark:hover:bg-slate-800/35">
                  <td className="px-4 py-3 font-mono font-semibold text-sm text-slate-900 dark:text-white">
                    {u.name}
                    {u.name === overview.currentUser && (
                      <span className="ml-2 px-1.5 py-0.5 rounded bg-indigo-500/15 text-indigo-500 text-[10px]">
                        YOU
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {u.isSuperuser && (
                        <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-amber-500/15 text-amber-600 dark:text-amber-300 text-[10px] font-semibold">
                          <Shield className="w-2.5 h-2.5" /> SUPERUSER
                        </span>
                      )}
                      {u.canLogin && (
                        <span className="px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 text-[10px] font-semibold">
                          LOGIN
                        </span>
                      )}
                      {u.canCreateDb && (
                        <span className="px-1.5 py-0.5 rounded bg-cyan-500/15 text-cyan-600 dark:text-cyan-300 text-[10px] font-semibold">
                          CREATEDB
                        </span>
                      )}
                      {u.canCreateRole && (
                        <span className="px-1.5 py-0.5 rounded bg-purple-500/15 text-purple-600 dark:text-purple-300 text-[10px] font-semibold">
                          CREATEROLE
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 font-mono">
                    {u.connLimit < 0 ? 'Unlimited' : u.connLimit}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {u.ownedDatabases.length > 0 ? (
                        u.ownedDatabases.map((db) => (
                          <span
                            key={db}
                            className="px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 font-mono text-[11px]"
                          >
                            {db}
                          </span>
                        ))
                      ) : (
                        <span className="text-slate-400">-</span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-1.5">
                      <button
                        type="button"
                        onClick={() => openAssignModal(u.name)}
                        className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg bg-emerald-600/15 hover:bg-emerald-600/25 text-emerald-700 dark:text-emerald-300 text-[11px] font-semibold"
                      >
                        <Database className="w-3 h-3" />
                        Assign DB
                      </button>
                      <button
                        type="button"
                        onClick={() => openEditModal(u)}
                        className="inline-flex items-center gap-1 px-2 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-[11px]"
                      >
                        <Edit3 className="w-3 h-3" />
                        Edit
                      </button>
                      {u.name !== overview.currentUser && (
                        <button
                          type="button"
                          onClick={() => setDropTarget(u.name)}
                          aria-label={`Drop ${u.name}`}
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
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500">
                    No database users found.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Create User Modal */}
      <Modal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Create Database User"
        subtitle="Create a new PostgreSQL/MySQL login role with custom privileges."
      >
        <form onSubmit={handleCreateUser} className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium mb-1">Username</label>
              <input
                type="text"
                required
                autoFocus
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="dadc_app"
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">Password</label>
              <input
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Strong password"
                className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
          </div>

          {overview.engine === 'postgres' && (
            <>
              <div className="grid grid-cols-2 gap-2.5 text-xs pt-1">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={canLogin} onChange={(e) => setCanLogin(e.target.checked)} />
                  Can Login (LOGIN)
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={isSuperuser} onChange={(e) => setIsSuperuser(e.target.checked)} />
                  Superuser
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={canCreateDb} onChange={(e) => setCanCreateDb(e.target.checked)} />
                  Create Databases
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={canCreateRole} onChange={(e) => setCanCreateRole(e.target.checked)} />
                  Create Roles
                </label>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Connection Limit (-1 = unlimited)</label>
                <input
                  type="number"
                  value={connLimit}
                  onChange={(e) => setConnLimit(Number(e.target.value))}
                  className="w-32 rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-1.5 text-xs font-mono"
                />
              </div>
            </>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={() => setCreateOpen(false)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold"
            >
              {submitting && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Create User
            </button>
          </div>
        </form>
      </Modal>

      {/* Edit User Modal */}
      <Modal
        open={Boolean(editTarget)}
        onClose={() => setEditTarget(null)}
        title={`Edit User — ${editTarget?.name ?? ''}`}
      >
        <form onSubmit={handleUpdateUser} className="space-y-4">
          <div>
            <label className="flex items-center gap-1.5 text-xs font-medium mb-1">
              <KeyRound className="w-3.5 h-3.5" /> New Password (leave blank to keep unchanged)
            </label>
            <input
              type="password"
              value={editPass}
              onChange={(e) => setEditPass(e.target.value)}
              placeholder="Enter new password..."
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm"
            />
          </div>

          {overview.engine === 'postgres' && (
            <>
              <div className="grid grid-cols-2 gap-2.5 text-xs">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={editLogin} onChange={(e) => setEditLogin(e.target.checked)} />
                  Can Login
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={editSuper} onChange={(e) => setEditSuper(e.target.checked)} />
                  Superuser
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={editCreateDb} onChange={(e) => setEditCreateDb(e.target.checked)} />
                  Create DB
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={editCreateRole} onChange={(e) => setEditCreateRole(e.target.checked)} />
                  Create Role
                </label>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Connection Limit</label>
                <input
                  type="number"
                  value={editConnLimit}
                  onChange={(e) => setEditConnLimit(Number(e.target.value))}
                  className="w-32 rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-1.5 text-xs font-mono"
                />
              </div>
            </>
          )}

          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setEditTarget(null)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold"
            >
              {submitting && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Save Changes
            </button>
          </div>
        </form>
      </Modal>

      {/* Assign User to Database Modal */}
      <Modal
        open={Boolean(assignTarget)}
        onClose={() => setAssignTarget(null)}
        title={`Assign "${assignTarget ?? ''}" to Database`}
        subtitle="Grants database, schema, and table permissions cleanly in one step."
      >
        <form onSubmit={handleAssignToDB} className="space-y-4">
          <div>
            <label className="block text-xs font-medium mb-1">Target Database</label>
            <select
              value={assignDB}
              onChange={(e) => setAssignDB(e.target.value)}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-sm font-mono"
            >
              {databases.map((d) => (
                <option key={d.name} value={d.name}>
                  {d.name}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium mb-1">Permission Preset</label>
            <select
              value={assignPreset}
              onChange={(e) => setAssignPreset(e.target.value)}
              className="w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 px-3 py-2 text-xs"
            >
              <option value="owner">Owner (Full Ownership of DB &amp; public schema)</option>
              <option value="admin">Admin (ALL privileges on DB, schema, tables, sequences)</option>
              <option value="read_write">Read &amp; Write (SELECT, INSERT, UPDATE, DELETE + sequences)</option>
              <option value="read_only">Read-Only (SELECT only on schema &amp; tables)</option>
              <option value="connect_only">Connect Only</option>
            </select>
          </div>

          {overview.capabilities.schemas && (
            <label className="flex items-center gap-2 text-xs cursor-pointer">
              <input
                type="checkbox"
                checked={assignDefaults}
                onChange={(e) => setAssignDefaults(e.target.checked)}
              />
              Apply default privileges for future tables
            </label>
          )}

          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => setAssignTarget(null)}
              className="px-4 py-2 rounded-xl border border-slate-200 dark:border-slate-700 text-xs"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting || !assignDB}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold"
            >
              {submitting && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Assign Permissions
            </button>
          </div>
        </form>
      </Modal>

      <ConfirmModal
        open={Boolean(dropTarget)}
        onClose={() => setDropTarget(null)}
        title={`Drop Database User — ${dropTarget ?? ''}`}
        confirmLabel="Drop User"
        loading={submitting}
        onConfirm={handleDropUser}
        message={
          <>
            Are you sure you want to permanently drop role{' '}
            <code className="font-mono font-semibold">{dropTarget}</code>? Roles that still own
            databases or objects must have ownership reassigned first.
          </>
        }
      />
    </div>
  );
}

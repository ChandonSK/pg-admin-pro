import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Loader2, Shield, Trash2, UserCheck, UserPlus } from 'lucide-react';
import { api } from '../api/client';
import type { DatabaseGrant, UserInfo } from '../types';

interface DatabaseGrantsPanelProps {
  dbName: string;
  users: UserInfo[];
  supportsSchemas: boolean;
  onNotify: (kind: 'success' | 'error' | 'info', msg: string) => void;
  onChanged?: () => void;
}

const PRESET_BADGE: Record<string, string> = {
  owner: 'bg-amber-500/15 text-amber-600 dark:text-amber-300 border-amber-500/30',
  admin: 'bg-indigo-500/15 text-indigo-600 dark:text-indigo-300 border-indigo-500/30',
  read_write: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300 border-emerald-500/30',
  read_only: 'bg-cyan-500/15 text-cyan-600 dark:text-cyan-300 border-cyan-500/30',
  connect: 'bg-slate-500/15 text-slate-600 dark:text-slate-300 border-slate-500/30',
  none: 'bg-slate-500/10 text-slate-400 border-slate-500/20',
};

export function DatabaseGrantsPanel({
  dbName,
  users,
  supportsSchemas,
  onNotify,
  onChanged,
}: DatabaseGrantsPanelProps) {
  const [grants, setGrants] = useState<DatabaseGrant[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const [selectedUser, setSelectedUser] = useState('');
  const [preset, setPreset] = useState('read_write');
  const [schema, setSchema] = useState('public');
  const [applyDefaults, setApplyDefaults] = useState(true);

  const loadGrants = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.databases.listGrants(dbName);
      setGrants(res.grants);
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to load grants');
    } finally {
      setLoading(false);
    }
  }, [dbName, onNotify]);

  useEffect(() => {
    loadGrants();
  }, [loadGrants]);

  useEffect(() => {
    if (!selectedUser && users.length > 0) {
      const firstLogin = users.find((u) => u.canLogin) || users[0];
      setSelectedUser(firstLogin.name);
    }
  }, [users, selectedUser]);

  const handleAssign = async (e: FormEvent) => {
    e.preventDefault();
    if (!selectedUser) return;
    setSaving(true);
    try {
      await api.databases.assignGrant(dbName, {
        username: selectedUser,
        preset,
        schema: supportsSchemas ? schema.trim() || 'public' : undefined,
        applyDefaultPrivs: applyDefaults,
      });
      onNotify('success', `Assigned ${selectedUser} (${preset}) to ${dbName}`);
      await loadGrants();
      onChanged?.();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to assign user');
    } finally {
      setSaving(false);
    }
  };

  const handleRevoke = async (username: string) => {
    setSaving(true);
    try {
      await api.databases.revokeGrant(dbName, username);
      onNotify('success', `Revoked ${username} access from ${dbName}`);
      await loadGrants();
      onChanged?.();
    } catch (err) {
      onNotify('error', err instanceof Error ? err.message : 'Failed to revoke access');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      {/* Assign User Form */}
      <form
        onSubmit={handleAssign}
        className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-950/60 p-4 space-y-3"
      >
        <div className="flex items-center gap-2 text-xs font-semibold text-slate-800 dark:text-slate-200">
          <UserPlus className="w-4 h-4 text-indigo-500" />
          Assign User / Role to <code className="font-mono text-indigo-500">{dbName}</code>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label className="block text-[11px] font-medium text-slate-500 mb-1">
              Database User
            </label>
            <select
              value={selectedUser}
              onChange={(e) => setSelectedUser(e.target.value)}
              className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-2 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
            >
              {users.map((u) => (
                <option key={u.name} value={u.name}>
                  {u.name} {u.isSuperuser ? '(super)' : ''}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-[11px] font-medium text-slate-500 mb-1">
              Permission Level
            </label>
            <select
              value={preset}
              onChange={(e) => setPreset(e.target.value)}
              className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-2 text-xs focus:outline-none focus:ring-2 focus:ring-indigo-500"
            >
              <option value="owner">Owner (Full Ownership)</option>
              <option value="admin">Admin (All Schema &amp; Tables)</option>
              <option value="read_write">Read &amp; Write (SELECT, INSERT, UPDATE, DELETE)</option>
              <option value="read_only">Read-Only (SELECT only)</option>
              {supportsSchemas && <option value="connect_only">Connect Only</option>}
            </select>
          </div>

          {supportsSchemas && (
            <div>
              <label className="block text-[11px] font-medium text-slate-500 mb-1">
                Target Schema
              </label>
              <input
                type="text"
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                className="w-full rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 px-2.5 py-2 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
          {supportsSchemas ? (
            <label className="inline-flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300 cursor-pointer">
              <input
                type="checkbox"
                checked={applyDefaults}
                onChange={(e) => setApplyDefaults(e.target.checked)}
                className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
              />
              Apply to future tables (<code>ALTER DEFAULT PRIVILEGES</code>)
            </label>
          ) : (
            <span />
          )}

          <button
            type="submit"
            disabled={saving || !selectedUser}
            className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-xs font-semibold transition-colors"
          >
            {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <UserCheck className="w-3.5 h-3.5" />}
            Assign Permissions
          </button>
        </div>
      </form>

      {/* Current Grants Table */}
      <div className="rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
        <div className="px-4 py-2.5 bg-slate-50 dark:bg-slate-900/80 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between">
          <span className="text-xs font-semibold flex items-center gap-1.5">
            <Shield className="w-3.5 h-3.5 text-emerald-500" />
            Current Role Permissions
          </span>
          {loading && <Loader2 className="w-3.5 h-3.5 animate-spin text-slate-400" />}
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-slate-50/70 dark:bg-slate-900/40 text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-800">
              <tr>
                <th className="px-4 py-2 font-medium">User</th>
                <th className="px-4 py-2 font-medium">Access Level</th>
                <th className="px-4 py-2 font-medium">Connect</th>
                <th className="px-4 py-2 font-medium">Create</th>
                <th className="px-4 py-2 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70">
              {grants.map((g) => (
                <tr key={g.username} className="hover:bg-slate-50 dark:hover:bg-slate-800/30">
                  <td className="px-4 py-2.5 font-mono font-semibold text-slate-900 dark:text-slate-100">
                    {g.username}
                  </td>
                  <td className="px-4 py-2.5">
                    <span
                      className={`inline-flex px-2 py-0.5 rounded-full text-[11px] font-medium border ${
                        PRESET_BADGE[g.schemaAccess] || PRESET_BADGE.none
                      }`}
                    >
                      {g.schemaAccess}
                    </span>
                  </td>
                  <td className="px-4 py-2.5">{g.canConnect ? 'Yes' : 'No'}</td>
                  <td className="px-4 py-2.5">{g.canCreate ? 'Yes' : 'No'}</td>
                  <td className="px-4 py-2.5 text-right">
                    {!g.isOwner && g.canConnect && (
                      <button
                        type="button"
                        disabled={saving}
                        onClick={() => handleRevoke(g.username)}
                        className="inline-flex items-center gap-1 px-2 py-1 rounded text-[11px] text-red-600 dark:text-red-400 hover:bg-red-500/10"
                      >
                        <Trash2 className="w-3 h-3" />
                        Revoke
                      </button>
                    )}
                    {g.isOwner && (
                      <span className="text-[11px] text-slate-400">Owner</span>
                    )}
                  </td>
                </tr>
              ))}
              {!loading && grants.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-6 text-center text-slate-500">
                    No login roles found.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

import type {
  ActivityOverview,
  AutocompleteTable,
  BackupFile,
  BackupResult,
  BackupSettingsUpdateReq,
  BackupSettingsView,
  ConnectionDefaults,
  DatabaseGrant,
  DatabaseInfo,
  DatabaseObjects,
  EngineKind,
  ExplainResult,
  FilterCond,
  ImportResult,
  ProfileEnv,
  ProfileUpsertReq,
  PublicConnectionInfo,
  QueryRowsResp,
  RestoreResult,
  S3Object,
  SchemaInfo,
  ServerOverview,
  ServerProfile,
  SQLResult,
  SQLSnippet,
  TableInfo,
  TableStructure,
  ToolCapabilities,
  UserInfo,
} from '../types';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const isFormData = typeof FormData !== 'undefined' && init?.body instanceof FormData;
  const headers: Record<string, string> = {
    'X-Requested-With': 'XMLHttpRequest',
    ...(init?.headers as Record<string, string> | undefined),
  };
  if (init?.body && !isFormData && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json';
  }
  const res = await fetch(path, {
    ...init,
    credentials: 'same-origin',
    headers,
  });

  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { error: text };
    }
  }

  if (!res.ok) {
    const errMsg =
      (data as { error?: string } | null)?.error || `Request failed (${res.status})`;
    throw new ApiError(res.status, errMsg);
  }
  return data as T;
}

export interface SessionPayload {
  connection: PublicConnectionInfo;
  overview: ServerOverview;
}

export const api = {
  auth: {
    defaults: () => request<ConnectionDefaults>('/api/auth/defaults'),
    session: () => request<SessionPayload>('/api/auth/session'),
    login: (payload: {
      engine?: EngineKind;
      host?: string;
      port?: number;
      database?: string;
      username?: string;
      password: string;
      sslMode?: string;
      profileId?: string;
      saveProfileName?: string;
      saveProfileEnv?: ProfileEnv;
      savePassword?: boolean;
    }) =>
      request<SessionPayload>('/api/auth/login', {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    logout: () => request<{ ok: boolean }>('/api/auth/logout', { method: 'POST' }),
  },
  profiles: {
    list: () => request<{ profiles: ServerProfile[] }>('/api/profiles'),
    create: (payload: ProfileUpsertReq) =>
      request<{ profile: ServerProfile }>('/api/profiles', {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    update: (id: string, payload: ProfileUpsertReq) =>
      request<{ profile: ServerProfile }>(`/api/profiles/${encodeURIComponent(id)}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      }),
    remove: (id: string) =>
      request<{ ok: boolean }>(`/api/profiles/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      }),
    switchTo: (id: string, password?: string) =>
      request<SessionPayload>(`/api/profiles/${encodeURIComponent(id)}/switch`, {
        method: 'POST',
        body: JSON.stringify(password ? { password } : {}),
      }),
  },
  snippets: {
    list: () => request<{ snippets: SQLSnippet[] }>('/api/snippets'),
    save: (payload: { id?: string; title: string; tag: string; sql: string; engine?: string }) =>
      request<{ snippet: SQLSnippet }>('/api/snippets', {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    remove: (id: string) =>
      request<{ ok: boolean }>(`/api/snippets/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      }),
  },
  overview: () => request<ServerOverview>('/api/overview'),
  databases: {
    list: () => request<{ databases: DatabaseInfo[] }>('/api/databases'),
    create: (payload: { name: string; owner?: string; encoding?: string }) =>
      request<{ ok: boolean }>('/api/databases', {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    drop: (name: string) =>
      request<{ ok: boolean }>(`/api/databases/${encodeURIComponent(name)}`, {
        method: 'DELETE',
      }),
    listGrants: (db: string) =>
      request<{ grants: DatabaseGrant[] }>(
        `/api/databases/${encodeURIComponent(db)}/grants`
      ),
    assignGrant: (
      db: string,
      payload: {
        username: string;
        preset: string;
        schema?: string;
        applyDefaultPrivs: boolean;
      }
    ) =>
      request<{ ok: boolean }>(`/api/databases/${encodeURIComponent(db)}/grants`, {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    revokeGrant: (db: string, username: string) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/grants/${encodeURIComponent(username)}`,
        { method: 'DELETE' }
      ),
  },
  users: {
    list: () => request<{ users: UserInfo[] }>('/api/users'),
    create: (payload: {
      username: string;
      password: string;
      isSuperuser: boolean;
      canLogin: boolean;
      canCreateDb: boolean;
      canCreateRole: boolean;
      connLimit: number;
    }) =>
      request<{ ok: boolean }>('/api/users', {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    update: (
      username: string,
      payload: {
        password?: string;
        isSuperuser?: boolean;
        canLogin?: boolean;
        canCreateDb?: boolean;
        canCreateRole?: boolean;
        connLimit?: number;
      }
    ) =>
      request<{ ok: boolean }>(`/api/users/${encodeURIComponent(username)}`, {
        method: 'PATCH',
        body: JSON.stringify(payload),
      }),
    drop: (username: string) =>
      request<{ ok: boolean }>(`/api/users/${encodeURIComponent(username)}`, {
        method: 'DELETE',
      }),
  },
  tables: {
    schemas: (db: string, includeSystem = false) =>
      request<{ schemas: SchemaInfo[] }>(
        `/api/databases/${encodeURIComponent(db)}/schemas?system=${includeSystem}`
      ),
    list: (db: string, schema: string) =>
      request<{ tables: TableInfo[] }>(
        `/api/databases/${encodeURIComponent(db)}/tables?schema=${encodeURIComponent(schema)}`
      ),
    structure: (db: string, schema: string, table: string) =>
      request<TableStructure>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/structure`
      ),
    create: (
      db: string,
      payload: {
        schema: string;
        table: string;
        columns: {
          name: string;
          type: string;
          nullable: boolean;
          primaryKey: boolean;
          default: string;
        }[];
      }
    ) =>
      request<{ ok: boolean }>(`/api/databases/${encodeURIComponent(db)}/tables`, {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    drop: (db: string, schema: string, table: string) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}`,
        { method: 'DELETE' }
      ),
    queryRows: (
      db: string,
      schema: string,
      table: string,
      payload: {
        page: number;
        pageSize: number;
        sortBy?: string;
        sortDir?: 'asc' | 'desc';
        search?: string;
        filters?: FilterCond[];
      }
    ) =>
      request<QueryRowsResp>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/query`,
        {
          method: 'POST',
          body: JSON.stringify(payload),
        }
      ),
    insertRow: (
      db: string,
      schema: string,
      table: string,
      values: Record<string, unknown>
    ) =>
      request<{ row: Record<string, unknown> }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/rows`,
        {
          method: 'POST',
          body: JSON.stringify({ values }),
        }
      ),
    updateRow: (
      db: string,
      schema: string,
      table: string,
      locator: Record<string, unknown>,
      updates: Record<string, unknown>
    ) =>
      request<{ row: Record<string, unknown> }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/rows`,
        {
          method: 'PATCH',
          body: JSON.stringify({ locator, updates }),
        }
      ),
    deleteRows: (
      db: string,
      schema: string,
      table: string,
      locators: Record<string, unknown>[]
    ) =>
      request<{ deleted: number }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/rows/delete`,
        {
          method: 'POST',
          body: JSON.stringify({ locators }),
        }
      ),
    addColumn: (
      db: string,
      schema: string,
      table: string,
      payload: { name: string; type: string; nullable: boolean; default: string }
    ) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/columns`,
        { method: 'POST', body: JSON.stringify(payload) }
      ),
    alterColumn: (
      db: string,
      schema: string,
      table: string,
      column: string,
      payload: {
        newName?: string;
        newType?: string;
        setNullable?: boolean;
        setDefault?: string;
        dropDefault?: boolean;
      }
    ) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/columns/${encodeURIComponent(column)}`,
        { method: 'PATCH', body: JSON.stringify(payload) }
      ),
    dropColumn: (db: string, schema: string, table: string, column: string) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/columns/${encodeURIComponent(column)}`,
        { method: 'DELETE' }
      ),
    truncate: (db: string, schema: string, table: string, cascade = false) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/truncate`,
        { method: 'POST', body: JSON.stringify({ cascade }) }
      ),
    createIndex: (
      db: string,
      schema: string,
      table: string,
      payload: { name: string; columns: string[]; unique: boolean; method: string }
    ) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/indexes`,
        { method: 'POST', body: JSON.stringify(payload) }
      ),
    dropIndex: (db: string, schema: string, table: string, name: string) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/indexes/${encodeURIComponent(name)}`,
        { method: 'DELETE' }
      ),
    addConstraint: (
      db: string,
      schema: string,
      table: string,
      payload: {
        name: string;
        type: 'FOREIGN KEY' | 'UNIQUE' | 'CHECK';
        columns: string[];
        refSchema?: string;
        refTable?: string;
        refColumns?: string[];
        onDelete?: string;
        onUpdate?: string;
        checkExpr?: string;
      }
    ) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/constraints`,
        { method: 'POST', body: JSON.stringify(payload) }
      ),
    dropConstraint: (db: string, schema: string, table: string, name: string) =>
      request<{ ok: boolean }>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/constraints/${encodeURIComponent(name)}`,
        { method: 'DELETE' }
      ),
    maintain: (
      db: string,
      schema: string,
      table: string,
      action: 'vacuum' | 'vacuum_analyze' | 'analyze' | 'reindex' | 'refresh_matview'
    ) =>
      request<SQLResult>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/maintenance`,
        { method: 'POST', body: JSON.stringify({ action }) }
      ),
    exportDownload: async (
      db: string,
      schema: string,
      table: string,
      payload: {
        format: 'csv' | 'json' | 'sql';
        scope: 'page' | 'all';
        page: number;
        pageSize: number;
        sortBy?: string;
        sortDir?: 'asc' | 'desc';
        search?: string;
        filters?: FilterCond[];
      }
    ) => {
      const res = await fetch(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/export`,
        {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/json',
            'X-Requested-With': 'XMLHttpRequest',
          },
          body: JSON.stringify(payload),
        }
      );
      if (!res.ok) {
        let msg = `Export failed (${res.status})`;
        try {
          const j = (await res.json()) as { error?: string };
          if (j?.error) msg = j.error;
        } catch {
          // ignore
        }
        throw new ApiError(res.status, msg);
      }
      const blob = await res.blob();
      const cd = res.headers.get('Content-Disposition') || '';
      const m = cd.match(/filename="([^"]+)"/);
      const fname = m?.[1] || `${table}.${payload.format}`;
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = fname;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    },
    importRows: (
      db: string,
      schema: string,
      table: string,
      payload: { rows: Record<string, unknown>[]; continueOnError: boolean }
    ) =>
      request<ImportResult>(
        `/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}/import`,
        { method: 'POST', body: JSON.stringify(payload) }
      ),
    objects: (db: string, schema = '') =>
      request<DatabaseObjects>(
        `/api/databases/${encodeURIComponent(db)}/objects?schema=${encodeURIComponent(schema)}`
      ),
    manageExtension: (db: string, name: string, action: 'install' | 'drop', schema = '') =>
      request<{ ok: boolean }>(`/api/databases/${encodeURIComponent(db)}/extensions`, {
        method: 'POST',
        body: JSON.stringify({ name, action, schema }),
      }),
    autocomplete: (db: string) =>
      request<{ tables: AutocompleteTable[] }>(
        `/api/databases/${encodeURIComponent(db)}/autocomplete`
      ),
  },
  sql: {
    execute: (db: string, sql: string) =>
      request<SQLResult>(`/api/databases/${encodeURIComponent(db)}/sql`, {
        method: 'POST',
        body: JSON.stringify({ sql }),
      }),
    explain: (db: string, sql: string, analyze = false) =>
      request<ExplainResult>(`/api/databases/${encodeURIComponent(db)}/explain`, {
        method: 'POST',
        body: JSON.stringify({ sql, analyze }),
      }),
  },
  monitor: {
    activity: (db = '') =>
      request<ActivityOverview>(
        `/api/monitor/activity${db ? `?db=${encodeURIComponent(db)}` : ''}`
      ),
    cancel: (pid: number) =>
      request<{ ok: boolean }>(`/api/monitor/activity/${pid}/cancel`, {
        method: 'POST',
      }),
    terminate: (pid: number) =>
      request<{ ok: boolean }>(`/api/monitor/activity/${pid}/terminate`, {
        method: 'POST',
      }),
  },
  backups: {
    list: () =>
      request<{ capabilities: ToolCapabilities; files: BackupFile[] }>('/api/backups'),
    create: (payload: { database: string; compression?: number; all?: boolean }) =>
      request<{ result?: BackupResult; results?: BackupResult[] }>(
        '/api/backups/create',
        {
          method: 'POST',
          body: JSON.stringify(payload),
        }
      ),
    upload: (file: File) => {
      const fd = new FormData();
      fd.append('file', file);
      return request<{ file: BackupFile }>('/api/backups/upload', {
        method: 'POST',
        body: fd,
      });
    },
    restore: (payload: {
      filename: string;
      targetDatabase: string;
      jobs: number;
      cleanFirst: boolean;
      createIfMissing: boolean;
    }) =>
      request<RestoreResult>('/api/backups/restore', {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    delete: (filename: string) =>
      request<{ ok: boolean }>(`/api/backups/${encodeURIComponent(filename)}`, {
        method: 'DELETE',
      }),
    downloadUrl: (filename: string) =>
      `/api/backups/download/${encodeURIComponent(filename)}`,
    getSettings: () =>
      request<{ settings: BackupSettingsView }>('/api/backups/settings'),
    updateSettings: (payload: BackupSettingsUpdateReq) =>
      request<{ settings: BackupSettingsView }>('/api/backups/settings', {
        method: 'PUT',
        body: JSON.stringify(payload),
      }),
    runScheduleNow: () =>
      request<{ results: BackupResult[]; settings: BackupSettingsView }>(
        '/api/backups/schedule/run',
        { method: 'POST' }
      ),
    pruneNow: () =>
      request<{ deleted: number }>('/api/backups/prune', { method: 'POST' }),
    testS3: () => request<{ ok: boolean }>('/api/backups/s3/test', { method: 'POST' }),
    listS3: () => request<{ objects: S3Object[] }>('/api/backups/s3/list'),
    pushS3: (filename: string) =>
      request<{ object: S3Object }>(
        `/api/backups/s3/push/${encodeURIComponent(filename)}`,
        { method: 'POST' }
      ),
    pullS3: (key: string) =>
      request<{ file: BackupFile }>('/api/backups/s3/pull', {
        method: 'POST',
        body: JSON.stringify({ key }),
      }),
    deleteS3: (key: string) =>
      request<{ ok: boolean }>(`/api/backups/s3?key=${encodeURIComponent(key)}`, {
        method: 'DELETE',
      }),
  },
};

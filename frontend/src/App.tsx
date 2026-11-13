import { useCallback, useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { api, type SessionPayload } from './api/client';
import { ActivityMonitorPage } from './components/ActivityMonitorPage';
import { BackupsPage } from './components/BackupsPage';
import { DatabasesPage } from './components/DatabasesPage';
import { DatabaseWorkspace } from './components/DatabaseWorkspace';
import { LoginView } from './components/LoginView';
import { Navbar, type NavTab } from './components/Navbar';
import { OverviewPage } from './components/OverviewPage';
import { ToastStack, type ToastItem } from './components/Toast';
import { UsersPage } from './components/UsersPage';
import type { DatabaseInfo, UserInfo } from './types';

export function App() {
  const [dark, setDark] = useState(() => {
    const saved = localStorage.getItem('pg_admin_pro_theme');
    return saved !== 'light';
  });

  const [checkingSession, setCheckingSession] = useState(true);
  const [session, setSession] = useState<SessionPayload | null>(null);
  const [activeTab, setActiveTab] = useState<NavTab>('overview');
  const [selectedDB, setSelectedDB] = useState('');

  const [databases, setDatabases] = useState<DatabaseInfo[]>([]);
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  useEffect(() => {
    const root = document.documentElement;
    if (dark) {
      root.classList.add('dark');
      localStorage.setItem('pg_admin_pro_theme', 'dark');
    } else {
      root.classList.remove('dark');
      localStorage.setItem('pg_admin_pro_theme', 'light');
    }
  }, [dark]);

  const notify = useCallback(
    (kind: ToastItem['kind'], message: string) => {
      const id = Date.now() + Math.floor(Math.random() * 1000);
      setToasts((prev) => [...prev.slice(-4), { id, kind, message }]);
      window.setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id));
      }, 4500);
    },
    []
  );

  const loadClusterData = useCallback(
    async (currentSession: SessionPayload) => {
      try {
        const [ov, dbRes] = await Promise.all([
          api.overview(),
          api.databases.list(),
        ]);
        setSession((prev) => (prev ? { ...prev, overview: ov } : currentSession));
        setDatabases(dbRes.databases);

        if (ov.capabilities.userManagement) {
          const uRes = await api.users.list();
          setUsers(uRes.users);
        } else {
          setUsers([]);
        }
      } catch (err) {
        notify('error', err instanceof Error ? err.message : 'Failed to refresh cluster data');
      }
    },
    [notify]
  );

  useEffect(() => {
    api.auth
      .session()
      .then(async (s) => {
        setSession(s);
        await loadClusterData(s);
      })
      .catch(() => {
        setSession(null);
      })
      .finally(() => setCheckingSession(false));
  }, [loadClusterData]);

  const handleLoginSuccess = async (s: SessionPayload) => {
    setSession(s);
    setActiveTab('overview');
    setSelectedDB('');
    await loadClusterData(s);
  };

  const handleSwitchSession = async (next: SessionPayload, label: string) => {
    setSession(next);
    setSelectedDB('');
    setActiveTab('overview');
    await loadClusterData(next);
    notify('success', `Switched server to "${label}"`);
  };

  const handleLogout = async () => {
    try {
      await api.auth.logout();
    } catch {
      // ignore
    }
    setSession(null);
    setDatabases([]);
    setUsers([]);
    setSelectedDB('');
  };

  const handleEnterDatabase = (dbName: string) => {
    setSelectedDB(dbName);
    setActiveTab('workspace');
  };

  if (checkingSession) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-950 text-slate-300">
        <div className="flex items-center gap-2 text-sm">
          <Loader2 className="w-5 h-5 animate-spin text-indigo-500" />
          Initializing pg-admin-pro...
        </div>
      </div>
    );
  }

  if (!session) {
    return (
      <>
        <LoginView
          onSuccess={handleLoginSuccess}
          dark={dark}
          onToggleTheme={() => setDark((d) => !d)}
        />
        <ToastStack
          toasts={toasts}
          onDismiss={(id) => setToasts((p) => p.filter((t) => t.id !== id))}
        />
      </>
    );
  }

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950 text-slate-900 dark:text-slate-100 flex flex-col">
      <Navbar
        activeTab={activeTab}
        onSelectTab={setActiveTab}
        connection={session.connection}
        overview={session.overview}
        databases={databases}
        selectedDatabase={selectedDB}
        onEnterDatabase={handleEnterDatabase}
        onSwitchSession={handleSwitchSession}
        onNotify={notify}
        onLogout={handleLogout}
        dark={dark}
        onToggleTheme={() => setDark((d) => !d)}
      />

      <main className="flex-1 max-w-[1600px] w-full mx-auto px-4 py-5">
        {activeTab === 'overview' && (
          <OverviewPage
            overview={session.overview}
            databases={databases}
            users={users}
            onNavigate={setActiveTab}
            onEnterDatabase={handleEnterDatabase}
            onRefresh={() => loadClusterData(session)}
          />
        )}

        {activeTab === 'databases' && (
          <DatabasesPage
            overview={session.overview}
            databases={databases}
            users={users}
            onRefresh={() => loadClusterData(session)}
            onEnterDatabase={handleEnterDatabase}
            onNotify={notify}
          />
        )}

        {activeTab === 'users' && (
          <UsersPage
            overview={session.overview}
            users={users}
            databases={databases}
            onRefresh={() => loadClusterData(session)}
            onNotify={notify}
          />
        )}

        {activeTab === 'monitor' && (
          <ActivityMonitorPage
            overview={session.overview}
            databases={databases}
            onNotify={notify}
          />
        )}

        {activeTab === 'backups' && (
          <BackupsPage
            overview={session.overview}
            databases={databases}
            onRefreshCluster={() => loadClusterData(session)}
            onNotify={notify}
          />
        )}

        {activeTab === 'workspace' && selectedDB && (
          <DatabaseWorkspace
            dbName={selectedDB}
            overview={session.overview}
            users={users}
            onBack={() => setActiveTab('databases')}
            onNotify={notify}
            onRefreshCluster={() => loadClusterData(session)}
          />
        )}
      </main>

      <ToastStack
        toasts={toasts}
        onDismiss={(id) => setToasts((p) => p.filter((t) => t.id !== id))}
      />
    </div>
  );
}

export default App;

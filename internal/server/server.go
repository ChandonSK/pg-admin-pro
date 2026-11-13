package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"pg-admin-pro/internal/auth"
	"pg-admin-pro/internal/backup"
	"pg-admin-pro/internal/config"
	"pg-admin-pro/internal/cryptoutil"
	"pg-admin-pro/internal/engine"
	"pg-admin-pro/internal/engine/factory"
	"pg-admin-pro/internal/store"
)

type ctxKey string

const driverCtxKey ctxKey = "driver"

// Server serves the REST API and React SPA from StaticDir.
type Server struct {
	cfg      *config.Config
	sessions *auth.Manager
	limiter  *auth.LoginLimiter
	backups  *backup.Manager
	profiles *store.ProfileStore
	snippets *store.SnippetStore
	mux      *http.ServeMux
}

// New creates an HTTP server wired with all API routes and static SPA serving.
func New(cfg *config.Config, sessions *auth.Manager, backups *backup.Manager) *Server {
	var (
		profiles *store.ProfileStore
		snippets *store.SnippetStore
	)
	if cfg != nil && cfg.DataDir != "" {
		if key, err := cryptoutil.LoadOrCreateKey(cfg.DataDir, cfg.ProfileEncKey); err == nil {
			profiles, _ = store.NewProfileStore(cfg.DataDir, key)
		}
		snippets, _ = store.NewSnippetStore(cfg.DataDir)
	}
	return NewWithStores(cfg, sessions, backups, profiles, snippets)
}

// NewWithStores creates an HTTP server using caller-supplied profile and snippet stores.
func NewWithStores(
	cfg *config.Config,
	sessions *auth.Manager,
	backups *backup.Manager,
	profiles *store.ProfileStore,
	snippets *store.SnippetStore,
) *Server {
	s := &Server{
		cfg:      cfg,
		sessions: sessions,
		limiter:  auth.NewLoginLimiter(),
		backups:  backups,
		profiles: profiles,
		snippets: snippets,
		mux:      http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the root HTTP handler with security headers.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "same-origin")
		s.mux.ServeHTTP(w, r)
	})
}

func (s *Server) registerRoutes() {
	// Public routes
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/auth/defaults", s.handleAuthDefaults)
	s.mux.HandleFunc("GET /api/profiles", s.handleListProfiles)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// Authenticated routes
	s.mux.HandleFunc("GET /api/auth/session", s.requireAuth(s.handleSession))
	s.mux.HandleFunc("GET /api/overview", s.requireAuth(s.handleOverview))

	// Saved server profiles
	s.mux.HandleFunc("POST /api/profiles", s.requireAuth(s.handleSaveProfile))
	s.mux.HandleFunc("PUT /api/profiles/{id}", s.requireAuth(s.handleUpdateProfile))
	s.mux.HandleFunc("DELETE /api/profiles/{id}", s.requireAuth(s.handleDeleteProfile))
	s.mux.HandleFunc("POST /api/profiles/{id}/switch", s.requireAuth(s.handleSwitchProfile))

	// Saved SQL snippets
	s.mux.HandleFunc("GET /api/snippets", s.requireAuth(s.handleListSnippets))
	s.mux.HandleFunc("POST /api/snippets", s.requireAuth(s.handleSaveSnippet))
	s.mux.HandleFunc("DELETE /api/snippets/{id}", s.requireAuth(s.handleDeleteSnippet))

	// Databases
	s.mux.HandleFunc("GET /api/databases", s.requireAuth(s.handleListDatabases))
	s.mux.HandleFunc("POST /api/databases", s.requireAuth(s.handleCreateDatabase))
	s.mux.HandleFunc("DELETE /api/databases/{db}", s.requireAuth(s.handleDropDatabase))

	// Users / Roles
	s.mux.HandleFunc("GET /api/users", s.requireAuth(s.handleListUsers))
	s.mux.HandleFunc("POST /api/users", s.requireAuth(s.handleCreateUser))
	s.mux.HandleFunc("PATCH /api/users/{username}", s.requireAuth(s.handleUpdateUser))
	s.mux.HandleFunc("DELETE /api/users/{username}", s.requireAuth(s.handleDropUser))

	// Database <-> User Grants
	s.mux.HandleFunc("GET /api/databases/{db}/grants", s.requireAuth(s.handleListGrants))
	s.mux.HandleFunc("POST /api/databases/{db}/grants", s.requireAuth(s.handleAssignGrant))
	s.mux.HandleFunc("DELETE /api/databases/{db}/grants/{username}", s.requireAuth(s.handleRevokeGrant))

	// Schemas & Tables
	s.mux.HandleFunc("GET /api/databases/{db}/schemas", s.requireAuth(s.handleListSchemas))
	s.mux.HandleFunc("GET /api/databases/{db}/tables", s.requireAuth(s.handleListTables))
	s.mux.HandleFunc("POST /api/databases/{db}/tables", s.requireAuth(s.handleCreateTable))
	s.mux.HandleFunc("GET /api/databases/{db}/tables/{schema}/{table}/structure", s.requireAuth(s.handleTableStructure))
	s.mux.HandleFunc("DELETE /api/databases/{db}/tables/{schema}/{table}", s.requireAuth(s.handleDropTable))

	// Table DDL Editor & Maintenance
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/columns", s.requireAuth(s.handleAddColumn))
	s.mux.HandleFunc("PATCH /api/databases/{db}/tables/{schema}/{table}/columns/{column}", s.requireAuth(s.handleAlterColumn))
	s.mux.HandleFunc("DELETE /api/databases/{db}/tables/{schema}/{table}/columns/{column}", s.requireAuth(s.handleDropColumn))
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/truncate", s.requireAuth(s.handleTruncateTable))
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/indexes", s.requireAuth(s.handleCreateIndex))
	s.mux.HandleFunc("DELETE /api/databases/{db}/tables/{schema}/{table}/indexes/{name}", s.requireAuth(s.handleDropIndex))
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/constraints", s.requireAuth(s.handleAddConstraint))
	s.mux.HandleFunc("DELETE /api/databases/{db}/tables/{schema}/{table}/constraints/{name}", s.requireAuth(s.handleDropConstraint))
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/maintenance", s.requireAuth(s.handleMaintainTable))

	// Row CRUD, Export/Import & SQL IDE
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/query", s.requireAuth(s.handleQueryRows))
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/rows", s.requireAuth(s.handleInsertRow))
	s.mux.HandleFunc("PATCH /api/databases/{db}/tables/{schema}/{table}/rows", s.requireAuth(s.handleUpdateRow))
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/rows/delete", s.requireAuth(s.handleDeleteRows))
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/export", s.requireAuth(s.handleExportRows))
	s.mux.HandleFunc("POST /api/databases/{db}/tables/{schema}/{table}/import", s.requireAuth(s.handleImportRows))
	s.mux.HandleFunc("POST /api/databases/{db}/sql", s.requireAuth(s.handleExecuteSQL))
	s.mux.HandleFunc("POST /api/databases/{db}/explain", s.requireAuth(s.handleExplainSQL))
	s.mux.HandleFunc("GET /api/databases/{db}/objects", s.requireAuth(s.handleDatabaseObjects))
	s.mux.HandleFunc("POST /api/databases/{db}/extensions", s.requireAuth(s.handleManageExtension))
	s.mux.HandleFunc("GET /api/databases/{db}/autocomplete", s.requireAuth(s.handleAutocompleteCatalog))

	// Activity & Locks Monitor
	s.mux.HandleFunc("GET /api/monitor/activity", s.requireAuth(s.handleActivityOverview))
	s.mux.HandleFunc("POST /api/monitor/activity/{pid}/cancel", s.requireAuth(s.handleCancelBackend))
	s.mux.HandleFunc("POST /api/monitor/activity/{pid}/terminate", s.requireAuth(s.handleTerminateBackend))

	// Backup & Restore Center + Scheduler + S3/R2 Sync
	s.mux.HandleFunc("GET /api/backups", s.requireAuth(s.handleListBackups))
	s.mux.HandleFunc("POST /api/backups/create", s.requireAuth(s.handleCreateBackup))
	s.mux.HandleFunc("POST /api/backups/restore", s.requireAuth(s.handleRestoreBackup))
	s.mux.HandleFunc("POST /api/backups/upload", s.requireAuth(s.handleUploadBackup))
	s.mux.HandleFunc("GET /api/backups/download/{filename}", s.requireAuth(s.handleDownloadBackup))
	s.mux.HandleFunc("DELETE /api/backups/{filename}", s.requireAuth(s.handleDeleteBackup))
	s.mux.HandleFunc("GET /api/backups/settings", s.requireAuth(s.handleGetBackupSettings))
	s.mux.HandleFunc("PUT /api/backups/settings", s.requireAuth(s.handleUpdateBackupSettings))
	s.mux.HandleFunc("POST /api/backups/schedule/run", s.requireAuth(s.handleRunScheduledBackup))
	s.mux.HandleFunc("POST /api/backups/prune", s.requireAuth(s.handlePruneBackups))
	s.mux.HandleFunc("POST /api/backups/s3/test", s.requireAuth(s.handleTestS3))
	s.mux.HandleFunc("GET /api/backups/s3/list", s.requireAuth(s.handleListS3Backups))
	s.mux.HandleFunc("POST /api/backups/s3/push/{filename}", s.requireAuth(s.handlePushBackupToS3))
	s.mux.HandleFunc("POST /api/backups/s3/pull", s.requireAuth(s.handlePullBackupFromS3))
	s.mux.HandleFunc("DELETE /api/backups/s3", s.requireAuth(s.handleDeleteS3Backup))

	// SPA Static File Fallback (must be last)
	s.mux.HandleFunc("/", s.handleSPA)
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("X-Requested-With") == "" {
				writeError(w, http.StatusForbidden, "missing X-Requested-With security header")
				return
			}
		}
		cookie, err := r.Cookie(auth.SessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		drv, ok := s.sessions.Get(cookie.Value)
		if !ok {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		ctx := context.WithValue(r.Context(), driverCtxKey, drv)
		next(w, r.WithContext(ctx))
	}
}

func getDriver(r *http.Request) engine.Driver {
	drv, _ := r.Context().Value(driverCtxKey).(engine.Driver)
	return drv
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthDefaults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"engine":   s.cfg.DefaultEngine,
		"host":     s.cfg.DefaultHost,
		"port":     s.cfg.DefaultPort,
		"database": s.cfg.DefaultDatabase,
		"username": s.cfg.DefaultUsername,
		"sslMode":  s.cfg.DefaultSSLMode,
	})
}

type loginReq struct {
	engine.ConnectionConfig
	ProfileID       string `json:"profileId"`
	SaveProfileName string `json:"saveProfileName"`
	SaveProfileEnv  string `json:"saveProfileEnv"`
	SavePassword    bool   `json:"savePassword"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.limiter.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts; please wait a minute")
		return
	}

	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	connCfg := req.ConnectionConfig
	if req.ProfileID != "" {
		if s.profiles == nil {
			writeError(w, http.StatusNotImplemented, "saved server profiles are unavailable")
			return
		}
		_, profCfg, err := s.profiles.GetConnection(req.ProfileID)
		if err != nil {
			writeError(w, http.StatusNotFound, "saved server profile not found")
			return
		}
		if req.Password != "" {
			profCfg.Password = req.Password
		}
		connCfg = profCfg
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	drv, err := factory.Connect(ctx, connCfg)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "connection failed: "+formatDBError(err))
		return
	}

	overview, err := drv.Overview(ctx)
	if err != nil {
		_ = drv.Close()
		writeError(w, http.StatusInternalServerError, "failed to load server overview: "+formatDBError(err))
		return
	}

	// Replace any existing cookie session cleanly
	if oldCookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		s.sessions.Delete(oldCookie.Value)
	}

	token, err := s.sessions.Create(drv)
	if err != nil {
		_ = drv.Close()
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	if s.profiles != nil {
		if req.ProfileID != "" {
			s.profiles.MarkUsed(req.ProfileID)
		} else if name := strings.TrimSpace(req.SaveProfileName); name != "" {
			_, _ = s.profiles.Upsert(store.ProfileUpsertReq{
				Name:         name,
				Env:          req.SaveProfileEnv,
				Engine:       connCfg.Engine,
				Host:         connCfg.Host,
				Port:         connCfg.Port,
				Database:     connCfg.Database,
				Username:     connCfg.Username,
				Password:     connCfg.Password,
				SavePassword: req.SavePassword,
				SSLMode:      connCfg.SSLMode,
			})
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"connection": drv.Config(),
		"overview":   overview,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		s.sessions.Delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	drv := getDriver(r)
	ov, err := drv.Overview(r.Context())
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connection": drv.Config(),
		"overview":   ov,
	})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := getDriver(r).Overview(r.Context())
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		writeJSON(w, http.StatusOK, map[string]any{"profiles": []store.ServerProfile{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": s.profiles.List()})
}

func (s *Server) handleSaveProfile(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		writeError(w, http.StatusNotImplemented, "profile store unavailable")
		return
	}
	var req store.ProfileUpsertReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := s.profiles.Upsert(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"profile": saved})
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		writeError(w, http.StatusNotImplemented, "profile store unavailable")
		return
	}
	var req store.ProfileUpsertReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.ID = r.PathValue("id")
	if !req.SavePassword && req.Password == "" {
		req.KeepPassword = true
	}
	saved, err := s.profiles.Upsert(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": saved})
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		writeError(w, http.StatusNotImplemented, "profile store unavailable")
		return
	}
	if err := s.profiles.Delete(r.PathValue("id")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSwitchProfile(w http.ResponseWriter, r *http.Request) {
	if s.profiles == nil {
		writeError(w, http.StatusNotImplemented, "profile store unavailable")
		return
	}
	id := r.PathValue("id")
	_, connCfg, err := s.profiles.GetConnection(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "saved server profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if body.Password != "" {
		connCfg.Password = body.Password
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	newDrv, err := factory.Connect(ctx, connCfg)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "switch connection failed: "+formatDBError(err))
		return
	}
	overview, err := newDrv.Overview(ctx)
	if err != nil {
		_ = newDrv.Close()
		writeError(w, http.StatusInternalServerError, "failed to load overview: "+formatDBError(err))
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || !s.sessions.Replace(cookie.Value, newDrv) {
		_ = newDrv.Close()
		writeError(w, http.StatusUnauthorized, "session expired")
		return
	}
	s.profiles.MarkUsed(id)

	writeJSON(w, http.StatusOK, map[string]any{
		"connection": newDrv.Config(),
		"overview":   overview,
	})
}

func (s *Server) handleListSnippets(w http.ResponseWriter, r *http.Request) {
	if s.snippets == nil {
		writeJSON(w, http.StatusOK, map[string]any{"snippets": []store.SQLSnippet{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snippets": s.snippets.List()})
}

func (s *Server) handleSaveSnippet(w http.ResponseWriter, r *http.Request) {
	if s.snippets == nil {
		writeError(w, http.StatusNotImplemented, "snippet store unavailable")
		return
	}
	var req store.SQLSnippet
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := s.snippets.Save(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snippet": item})
}

func (s *Server) handleDeleteSnippet(w http.ResponseWriter, r *http.Request) {
	if s.snippets == nil {
		writeError(w, http.StatusNotImplemented, "snippet store unavailable")
		return
	}
	if err := s.snippets.Delete(r.PathValue("id")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "snippet not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	dbs, err := getDriver(r).ListDatabases(r.Context())
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"databases": dbs})
}

func (s *Server) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	var req engine.CreateDatabaseReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := getDriver(r).CreateDatabase(r.Context(), req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) handleDropDatabase(w http.ResponseWriter, r *http.Request) {
	dbName := r.PathValue("db")
	if err := getDriver(r).DropDatabase(r.Context(), dbName); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := getDriver(r).ListUsers(r.Context())
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req engine.CreateUserReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := getDriver(r).CreateUser(r.Context(), req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	var req engine.UpdateUserReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := getDriver(r).UpdateUser(r.Context(), username, req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDropUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if err := getDriver(r).DropUser(r.Context(), username); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListGrants(w http.ResponseWriter, r *http.Request) {
	dbName := r.PathValue("db")
	grants, err := getDriver(r).ListDatabaseGrants(r.Context(), dbName)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"grants": grants})
}

func (s *Server) handleAssignGrant(w http.ResponseWriter, r *http.Request) {
	dbName := r.PathValue("db")
	var req engine.AssignUserReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := getDriver(r).AssignUserToDatabase(r.Context(), dbName, req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRevokeGrant(w http.ResponseWriter, r *http.Request) {
	dbName := r.PathValue("db")
	username := r.PathValue("username")
	if err := getDriver(r).RevokeUserFromDatabase(r.Context(), dbName, username); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListSchemas(w http.ResponseWriter, r *http.Request) {
	dbName := r.PathValue("db")
	includeSys := r.URL.Query().Get("system") == "true"
	schemas, err := getDriver(r).ListSchemas(r.Context(), dbName, includeSys)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schemas": schemas})
}

func (s *Server) handleListTables(w http.ResponseWriter, r *http.Request) {
	dbName := r.PathValue("db")
	schema := r.URL.Query().Get("schema")
	tables, err := getDriver(r).ListTables(r.Context(), dbName, schema)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

func (s *Server) handleCreateTable(w http.ResponseWriter, r *http.Request) {
	dbName := r.PathValue("db")
	var req engine.CreateTableReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := getDriver(r).CreateTable(r.Context(), dbName, req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) handleTableStructure(w http.ResponseWriter, r *http.Request) {
	st, err := getDriver(r).GetTableStructure(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"))
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleDropTable(w http.ResponseWriter, r *http.Request) {
	if err := getDriver(r).DropTable(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table")); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleQueryRows(w http.ResponseWriter, r *http.Request) {
	var req engine.QueryRowsReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Schema = r.PathValue("schema")
	req.Table = r.PathValue("table")
	resp, err := getDriver(r).QueryRows(r.Context(), r.PathValue("db"), req)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleInsertRow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Values map[string]any `json:"values"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := getDriver(r).InsertRow(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), body.Values)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"row": row})
}

func (s *Server) handleUpdateRow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Locator map[string]any `json:"locator"`
		Updates map[string]any `json:"updates"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := getDriver(r).UpdateRow(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), body.Locator, body.Updates)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"row": row})
}

func (s *Server) handleDeleteRows(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Locators []map[string]any `json:"locators"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	deleted, err := getDriver(r).DeleteRows(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), body.Locators)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": deleted})
}

func (s *Server) handleExecuteSQL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SQL string `json:"sql"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := getDriver(r).ExecuteSQL(r.Context(), r.PathValue("db"), body.SQL)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleAddColumn(w http.ResponseWriter, r *http.Request) {
	var req engine.AddColumnReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Schema = r.PathValue("schema")
	req.Table = r.PathValue("table")
	if err := getDriver(r).AddColumn(r.Context(), r.PathValue("db"), req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) handleAlterColumn(w http.ResponseWriter, r *http.Request) {
	var req engine.AlterColumnReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Schema = r.PathValue("schema")
	req.Table = r.PathValue("table")
	req.Column = r.PathValue("column")
	if err := getDriver(r).AlterColumn(r.Context(), r.PathValue("db"), req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDropColumn(w http.ResponseWriter, r *http.Request) {
	if err := getDriver(r).DropColumn(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), r.PathValue("column")); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTruncateTable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Cascade bool `json:"cascade"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := getDriver(r).TruncateTable(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), body.Cascade); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleCreateIndex(w http.ResponseWriter, r *http.Request) {
	var req engine.CreateIndexReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Schema = r.PathValue("schema")
	req.Table = r.PathValue("table")
	if err := getDriver(r).CreateIndex(r.Context(), r.PathValue("db"), req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) handleDropIndex(w http.ResponseWriter, r *http.Request) {
	if err := getDriver(r).DropIndex(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), r.PathValue("name")); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAddConstraint(w http.ResponseWriter, r *http.Request) {
	var req engine.AddConstraintReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Schema = r.PathValue("schema")
	req.Table = r.PathValue("table")
	if err := getDriver(r).AddConstraint(r.Context(), r.PathValue("db"), req); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) handleDropConstraint(w http.ResponseWriter, r *http.Request) {
	if err := getDriver(r).DropConstraint(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), r.PathValue("name")); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMaintainTable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := getDriver(r).MaintainTable(r.Context(), r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), body.Action)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExportRows(w http.ResponseWriter, r *http.Request) {
	var body struct {
		engine.QueryRowsReq
		Format string `json:"format"` // csv, json, sql
		Scope  string `json:"scope"`  // page, all
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body.Schema = r.PathValue("schema")
	body.Table = r.PathValue("table")
	format := strings.ToLower(strings.TrimSpace(body.Format))
	if format == "" {
		format = "csv"
	}
	switch format {
	case "csv", "json", "sql":
	default:
		writeError(w, http.StatusBadRequest, "export format must be csv, json, or sql")
		return
	}

	allRows := strings.EqualFold(body.Scope, "all")
	drv := getDriver(r)
	eng := drv.Config().Engine
	safeTable := strings.ReplaceAll(body.Table, `"`, "")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	var (
		colsMeta      []engine.ColumnInfo
		headersSent   bool
		rowIdx        int
		csvWriter     *csv.Writer
		sqlHeaderPrefix string
	)

	onHeader := func(cols []engine.ColumnInfo) error {
		colsMeta = cols
		headersSent = true
		switch format {
		case "csv":
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeTable+".csv"))
			csvWriter = csv.NewWriter(w)
			hdr := make([]string, len(cols))
			for i, c := range cols {
				hdr[i] = c.Name
			}
			return csvWriter.Write(hdr)
		case "json":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeTable+".json"))
			_, err := io.WriteString(w, "[\n")
			return err
		case "sql":
			w.Header().Set("Content-Type", "application/sql; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", safeTable+".sql"))
			colParts := make([]string, len(cols))
			for i, c := range cols {
				colParts[i] = quoteExportIdent(c.Name, eng)
			}
			tblRef := quoteExportIdent(body.Table, eng)
			if body.Schema != "" && eng == engine.EnginePostgres {
				tblRef = quoteExportIdent(body.Schema, eng) + "." + tblRef
			}
			sqlHeaderPrefix = fmt.Sprintf("INSERT INTO %s (%s) VALUES ", tblRef, strings.Join(colParts, ", "))
			_, err := fmt.Fprintf(w, "-- Exported %s.%s via pg-admin-pro\n", body.Schema, body.Table)
			return err
		}
		return nil
	}

	onRow := func(vals []any) error {
		switch format {
		case "csv":
			rec := make([]string, len(vals))
			for i, v := range vals {
				rec[i] = formatExportCellCSV(v)
			}
			if err := csvWriter.Write(rec); err != nil {
				return err
			}
			rowIdx++
			if rowIdx%500 == 0 {
				csvWriter.Flush()
				return csvWriter.Error()
			}
			return nil
		case "json":
			obj := make(map[string]any, len(colsMeta))
			for i, c := range colsMeta {
				if i < len(vals) {
					obj[c.Name] = vals[i]
				}
			}
			encoded, err := json.Marshal(obj)
			if err != nil {
				return err
			}
			prefix := ""
			if rowIdx > 0 {
				prefix = ",\n"
			}
			rowIdx++
			_, err = w.Write(append([]byte(prefix), encoded...))
			return err
		case "sql":
			literals := make([]string, len(vals))
			for i, v := range vals {
				literals[i] = formatExportCellSQL(v, eng)
			}
			rowIdx++
			_, err := fmt.Fprintf(w, "%s(%s);\n", sqlHeaderPrefix, strings.Join(literals, ", "))
			return err
		}
		return nil
	}

	if err := drv.StreamRows(ctx, r.PathValue("db"), body.QueryRowsReq, allRows, onHeader, onRow); err != nil {
		if !headersSent {
			writeDBError(w, err)
		}
		return
	}

	if format == "csv" && csvWriter != nil {
		csvWriter.Flush()
	} else if format == "json" {
		if !headersSent {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = io.WriteString(w, "[]\n")
		} else {
			_, _ = io.WriteString(w, "\n]\n")
		}
	}
}

func quoteExportIdent(ident string, eng engine.EngineKind) string {
	if eng == engine.EngineMySQL {
		return "`" + strings.ReplaceAll(ident, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func formatExportCellCSV(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339Nano)
	case bool, int, int64, int32, float64, float32:
		return fmt.Sprintf("%v", t)
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(raw)
	}
}

func formatExportCellSQL(v any, eng engine.EngineKind) string {
	if v == nil {
		return "NULL"
	}
	switch t := v.(type) {
	case bool:
		if t {
			return "TRUE"
		}
		return "FALSE"
	case int, int32, int64, uint64, float32, float64:
		return fmt.Sprintf("%v", t)
	case time.Time:
		return "'" + t.UTC().Format("2006-01-02 15:04:05.999999-07:00") + "'"
	case string:
		s := strings.ReplaceAll(t, "\x00", "")
		if eng == engine.EngineMySQL {
			s = strings.ReplaceAll(s, `\`, `\\`)
		}
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	case []byte:
		s := strings.ReplaceAll(string(t), "\x00", "")
		if eng == engine.EngineMySQL {
			s = strings.ReplaceAll(s, `\`, `\\`)
		}
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return "'" + strings.ReplaceAll(fmt.Sprintf("%v", t), "'", "''") + "'"
		}
		s := string(raw)
		if eng == engine.EngineMySQL {
			s = strings.ReplaceAll(s, `\`, `\\`)
		}
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
}

func (s *Server) handleImportRows(w http.ResponseWriter, r *http.Request) {
	var req engine.ImportRowsReq
	if err := decodeJSONLimit(r, &req, 32<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	res, err := getDriver(r).ImportRows(ctx, r.PathValue("db"), r.PathValue("schema"), r.PathValue("table"), req)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleExplainSQL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SQL     string `json:"sql"`
		Analyze bool   `json:"analyze"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	res, err := getDriver(r).ExplainSQL(ctx, r.PathValue("db"), body.SQL, body.Analyze)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDatabaseObjects(w http.ResponseWriter, r *http.Request) {
	objs, err := getDriver(r).ListDatabaseObjects(r.Context(), r.PathValue("db"), r.URL.Query().Get("schema"))
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, objs)
}

func (s *Server) handleManageExtension(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Schema string `json:"schema"`
		Action string `json:"action"` // install, drop
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := getDriver(r).ManageExtension(r.Context(), r.PathValue("db"), body.Name, body.Schema, body.Action); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAutocompleteCatalog(w http.ResponseWriter, r *http.Request) {
	tables, err := getDriver(r).ListAutocompleteCatalog(r.Context(), r.PathValue("db"))
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

func (s *Server) handleActivityOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := getDriver(r).GetActivityOverview(r.Context(), r.URL.Query().Get("db"))
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

func (s *Server) handleCancelBackend(w http.ResponseWriter, r *http.Request) {
	s.handleBackendSignal(w, r, false)
}

func (s *Server) handleTerminateBackend(w http.ResponseWriter, r *http.Request) {
	s.handleBackendSignal(w, r, true)
}

func (s *Server) handleBackendSignal(w http.ResponseWriter, r *http.Request, terminate bool) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 0 {
		writeError(w, http.StatusBadRequest, "invalid backend pid")
		return
	}
	if err := getDriver(r).CancelBackend(r.Context(), pid, terminate); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	caps := s.backups.Capabilities(r.Context())
	files, err := s.backups.ListBackups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"capabilities": caps,
		"files":        files,
	})
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Database    string `json:"database"`
		Compression int    `json:"compression"`
		All         bool   `json:"all"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Compression == 0 {
		body.Compression = 6
	}
	drv := getDriver(r)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	if body.All {
		dbs, err := drv.ListDatabases(ctx)
		if err != nil {
			writeDBError(w, err)
			return
		}
		var results []*backup.BackupResult
		for _, db := range dbs {
			if !db.AllowConnect {
				continue
			}
			res, err := s.backups.CreateBackup(ctx, drv, db.Name, body.Compression)
			if err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("backup failed on %s: %v", db.Name, err))
				return
			}
			results = append(results, res)
		}
		writeJSON(w, http.StatusCreated, map[string]any{"results": results})
		return
	}

	res, err := s.backups.CreateBackup(ctx, drv, body.Database, body.Compression)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"result": res})
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Filename        string `json:"filename"`
		TargetDatabase  string `json:"targetDatabase"`
		Jobs            int    `json:"jobs"`
		CleanFirst      bool   `json:"cleanFirst"`
		CreateIfMissing bool   `json:"createIfMissing"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	res, err := s.backups.RestoreBackup(ctx, getDriver(r), body.Filename, body.TargetDatabase, body.Jobs, body.CleanFirst, body.CreateIfMissing)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	rc, size, err := s.backups.OpenBackup(filename)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	_, _ = io.Copy(w, rc)
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if err := s.backups.DeleteBackup(filename); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleUploadBackup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected multipart/form-data file upload: "+err.Error())
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if part.FileName() == "" {
			_ = part.Close()
			continue
		}
		file, saveErr := s.backups.SaveUploadedBackup(part.FileName(), part)
		_ = part.Close()
		if saveErr != nil {
			writeDBError(w, saveErr)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"file": file})
		return
	}
	writeError(w, http.StatusBadRequest, "no backup file provided in upload")
}

func (s *Server) handleGetBackupSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.backups.GetSettings()})
}

func (s *Server) handleUpdateBackupSettings(w http.ResponseWriter, r *http.Request) {
	var req backup.BackupSettingsUpdateReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	settings, err := s.backups.UpdateSettings(req)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) handleRunScheduledBackup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	results, err := s.backups.RunScheduledNow(ctx, getDriver(r))
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results":  results,
		"settings": s.backups.GetSettings(),
	})
}

func (s *Server) handlePruneBackups(w http.ResponseWriter, r *http.Request) {
	deleted, err := s.backups.PruneBackups()
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}

func (s *Server) handleTestS3(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if err := s.backups.TestS3(ctx); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListS3Backups(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	objects, err := s.backups.ListS3Backups(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"objects": objects})
}

func (s *Server) handlePushBackupToS3(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	obj, err := s.backups.PushBackupToS3(ctx, r.PathValue("filename"))
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": obj})
}

func (s *Server) handlePullBackupFromS3(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	file, err := s.backups.PullBackupFromS3(ctx, body.Key)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file": file})
}

func (s *Server) handleDeleteS3Backup(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		var body struct {
			Key string `json:"key"`
		}
		if r.ContentLength > 0 {
			_ = decodeJSON(r, &body)
			key = body.Key
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if err := s.backups.DeleteS3Backup(ctx, key); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "API endpoint not found")
		return
	}

	cleanPath := filepath.Clean("/" + r.URL.Path)
	relPath := strings.TrimPrefix(cleanPath, "/")
	staticAbs, err := filepath.Abs(s.cfg.StaticDir)
	if err != nil {
		http.Error(w, "invalid static dir", http.StatusInternalServerError)
		return
	}

	target := filepath.Join(staticAbs, relPath)
	if !strings.HasPrefix(target, staticAbs) {
		http.NotFound(w, r)
		return
	}

	if relPath != "" {
		if fi, statErr := os.Stat(target); statErr == nil && !fi.IsDir() {
			if strings.HasPrefix(relPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFile(w, r, target)
			return
		}
	}

	indexPath := filepath.Join(staticAbs, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<!doctype html><html><body style="font-family:system-ui;background:#0f172a;color:#e2e8f0;padding:40px"><h2>pg-admin-pro backend is running</h2><p>Frontend assets not built yet in <code>`+s.cfg.StaticDir+`</code>. Run <code>cd frontend && npm run build</code>.</p></body></html>`)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, indexPath)
}

func decodeJSON(r *http.Request, dst any) error {
	return decodeJSONLimit(r, dst, 4<<20)
}

func decodeJSONLimit(r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeDBError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, engine.ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, engine.ErrUnsupported):
		status = http.StatusNotImplemented
	}
	writeError(w, status, formatDBError(err))
}

func formatDBError(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		msg := pgErr.Message
		if pgErr.Detail != "" {
			msg += " — " + pgErr.Detail
		}
		if pgErr.Hint != "" {
			msg += " (Hint: " + pgErr.Hint + ")"
		}
		return msg
	}
	return err.Error()
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

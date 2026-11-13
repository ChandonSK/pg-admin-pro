package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pg-admin-pro/internal/auth"
	"pg-admin-pro/internal/backup"
	"pg-admin-pro/internal/config"
	"pg-admin-pro/internal/server"
)

func TestServerAuthCSRFAndCRUD(t *testing.T) {
	tmp := t.TempDir()
	webDir := filepath.Join(tmp, "web")
	backupDir := filepath.Join(tmp, "backups")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<html>pg-admin-pro</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		StaticDir:       webDir,
		BackupDir:       backupDir,
		SessionTTL:      time.Hour,
		DefaultEngine:   "postgres",
		DefaultHost:     "127.0.0.1",
		DefaultPort:     5432,
		DefaultDatabase: "postgres",
		DefaultUsername: "postgres",
		DefaultSSLMode:  "disable",
	}

	sessions := auth.NewManager(cfg.SessionTTL)
	defer sessions.Close()

	srv := server.New(cfg, sessions, backup.NewManager(cfg))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// 1. Health & SPA served
	resp, err := client.Get(ts.URL + "/api/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("health check failed: %v, status=%v", err, resp)
	}
	resp.Body.Close()

	resp, err = client.Get(ts.URL + "/")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("SPA root failed: %v", err)
	}
	resp.Body.Close()

	// 2. Unauthenticated overview must return 401
	resp, err = client.Get(ts.URL + "/api/overview")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated overview, got %v", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Login with SQLite temp file
	sqliteDB := filepath.Join(tmp, "api_test.db")
	loginBody, _ := json.Marshal(map[string]any{
		"engine":   "sqlite",
		"database": sqliteDB,
	})
	resp, err = client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %v, status=%v", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Mutating request WITHOUT X-Requested-With header must be blocked (403 CSRF check)
	createTblBody, _ := json.Marshal(map[string]any{
		"schema": "main",
		"table":  "items",
		"columns": []map[string]any{
			{"name": "id", "type": "INTEGER", "primaryKey": true, "nullable": false},
			{"name": "title", "type": "TEXT", "nullable": false},
		},
	})
	reqNoCSRF, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/databases/api_test.db/tables", bytes.NewReader(createTblBody))
	reqNoCSRF.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(reqNoCSRF)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 when X-Requested-With header missing, got %v", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Mutating request WITH X-Requested-With succeeds
	reqWithCSRF, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/databases/api_test.db/tables", bytes.NewReader(createTblBody))
	reqWithCSRF.Header.Set("Content-Type", "application/json")
	reqWithCSRF.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err = client.Do(reqWithCSRF)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating table, got %v", resp.StatusCode)
	}
	resp.Body.Close()

	// 6. Logout invalidates session
	reqLogout, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth/logout", nil)
	reqLogout.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err = client.Do(reqLogout)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("logout failed: %v", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = client.Get(ts.URL + "/api/overview")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %v", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestServerExtendedFeatures(t *testing.T) {
	tmp := t.TempDir()
	webDir := filepath.Join(tmp, "web")
	backupDir := filepath.Join(tmp, "backups")
	dataDir := filepath.Join(tmp, "data")
	for _, dir := range []string{webDir, backupDir, dataDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<html>ok</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		StaticDir:  webDir,
		BackupDir:  backupDir,
		DataDir:    dataDir,
		SessionTTL: time.Hour,
	}
	sessions := auth.NewManager(cfg.SessionTTL)
	defer sessions.Close()

	srv := server.New(cfg, sessions, backup.NewManager(cfg))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	doJSON := func(method, path string, payload any) *http.Response {
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		r, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", method, path, err)
		}
		return r
	}

	// Login while saving a profile
	sqliteDB := filepath.Join(tmp, "ext_api.db")
	resp := doJSON(http.MethodPost, "/api/auth/login", map[string]any{
		"engine":          "sqlite",
		"database":        sqliteDB,
		"saveProfileName": "Local SQLite",
		"saveProfileEnv":  "local",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify profile saved & switchable without logging out
	profResp, err := client.Get(ts.URL + "/api/profiles")
	if err != nil || profResp.StatusCode != http.StatusOK {
		t.Fatalf("profiles list failed: %v", err)
	}
	var profPayload struct {
		Profiles []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"profiles"`
	}
	_ = json.NewDecoder(profResp.Body).Decode(&profPayload)
	profResp.Body.Close()
	if len(profPayload.Profiles) != 1 || profPayload.Profiles[0].Name != "Local SQLite" {
		t.Fatalf("unexpected profiles: %+v", profPayload.Profiles)
	}

	swResp := doJSON(http.MethodPost, "/api/profiles/"+profPayload.Profiles[0].ID+"/switch", map[string]any{})
	if swResp.StatusCode != http.StatusOK {
		t.Fatalf("profile switch failed: %d", swResp.StatusCode)
	}
	swResp.Body.Close()

	// Create table + Add column + Import + Export CSV + Snippets + Explain
	if r := doJSON(http.MethodPost, "/api/databases/ext_api.db/tables", map[string]any{
		"schema": "main",
		"table":  "courses",
		"columns": []map[string]any{
			{"name": "id", "type": "INTEGER", "primaryKey": true},
			{"name": "code", "type": "TEXT"},
		},
	}); r.StatusCode != http.StatusCreated {
		t.Fatalf("create table failed: %d", r.StatusCode)
	} else {
		r.Body.Close()
	}

	if r := doJSON(http.MethodPost, "/api/databases/ext_api.db/tables/main/courses/columns", map[string]any{
		"name": "title", "type": "TEXT", "nullable": true,
	}); r.StatusCode != http.StatusCreated {
		t.Fatalf("add column failed: %d", r.StatusCode)
	} else {
		r.Body.Close()
	}

	if r := doJSON(http.MethodPost, "/api/databases/ext_api.db/tables/main/courses/import", map[string]any{
		"rows": []map[string]any{
			{"id": 1, "code": "CSE101", "title": "Intro"},
			{"id": 2, "code": "MAT201", "title": "Calculus"},
		},
	}); r.StatusCode != http.StatusOK {
		t.Fatalf("import rows failed: %d", r.StatusCode)
	} else {
		r.Body.Close()
	}

	expResp := doJSON(http.MethodPost, "/api/databases/ext_api.db/tables/main/courses/export", map[string]any{
		"format": "csv",
		"scope":  "all",
	})
	if expResp.StatusCode != http.StatusOK || !bytes.Contains([]byte(expResp.Header.Get("Content-Type")), []byte("text/csv")) {
		t.Fatalf("export csv failed: status=%d ct=%s", expResp.StatusCode, expResp.Header.Get("Content-Type"))
	}
	expResp.Body.Close()

	snipResp := doJSON(http.MethodPost, "/api/snippets", map[string]any{
		"title": "All Courses",
		"tag":   "queries",
		"sql":   "SELECT * FROM courses;",
	})
	if snipResp.StatusCode != http.StatusOK {
		t.Fatalf("save snippet failed: %d", snipResp.StatusCode)
	}
	snipResp.Body.Close()

	// Backup settings update + prune
	bsResp := doJSON(http.MethodPut, "/api/backups/settings", map[string]any{
		"autoBackupEnabled": true,
		"autoBackupTime":    "03:30",
		"retentionDays":     7,
		"maxBackups":        5,
	})
	if bsResp.StatusCode != http.StatusOK {
		t.Fatalf("update backup settings failed: %d", bsResp.StatusCode)
	}
	bsResp.Body.Close()
}

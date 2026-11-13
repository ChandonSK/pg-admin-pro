package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pg-admin-pro/internal/config"
	"pg-admin-pro/internal/cryptoutil"
	"pg-admin-pro/internal/engine"
	"pg-admin-pro/internal/engine/mysql"
	"pg-admin-pro/internal/engine/postgres"
	"pg-admin-pro/internal/engine/sqlite"
	"pg-admin-pro/internal/store"
)

var (
	safeFilenameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,180}$`)
	hhmmRe         = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// ToolCapabilities reports whether native CLI backup tools exist on this host.
type ToolCapabilities struct {
	PgDumpAvailable      bool   `json:"pgDumpAvailable"`
	PgDumpVersion        string `json:"pgDumpVersion,omitempty"`
	PgRestoreAvailable   bool   `json:"pgRestoreAvailable"`
	PgRestoreVersion     string `json:"pgRestoreVersion,omitempty"`
	PsqlAvailable        bool   `json:"psqlAvailable"`
	MySQLDumpAvailable   bool   `json:"mysqlDumpAvailable"`
	MySQLClientAvailable bool   `json:"mysqlClientAvailable"`
	S3Configured         bool   `json:"s3Configured"`
	BackupDir            string `json:"backupDir"`
}

// BackupSettingsView exposes non-secret scheduled backup & S3 sync settings to the UI.
type BackupSettingsView struct {
	AutoBackupEnabled  bool   `json:"autoBackupEnabled"`
	AutoBackupTime     string `json:"autoBackupTime"`
	AutoBackupDatabase string `json:"autoBackupDatabase"`
	Compression        int    `json:"compression"`
	RetentionDays      int    `json:"retentionDays"`
	MaxBackups         int    `json:"maxBackups"`
	AutoHost           string `json:"autoHost"`
	AutoPort           int    `json:"autoPort"`
	AutoDatabase       string `json:"autoDatabase"`
	AutoUsername       string `json:"autoUsername"`
	AutoSSLMode        string `json:"autoSslMode"`
	HasAutoPassword    bool   `json:"hasAutoPassword"`
	S3Enabled          bool   `json:"s3Enabled"`
	S3Endpoint         string `json:"s3Endpoint"`
	S3Region           string `json:"s3Region"`
	S3Bucket           string `json:"s3Bucket"`
	S3Prefix           string `json:"s3Prefix"`
	S3AccessKeyID      string `json:"s3AccessKeyId"`
	HasS3SecretKey     bool   `json:"hasS3SecretKey"`
	S3ForcePathStyle   bool   `json:"s3ForcePathStyle"`
	S3AutoUpload       bool   `json:"s3AutoUpload"`
	LastRunAt          string `json:"lastRunAt,omitempty"`
	LastRunStatus      string `json:"lastRunStatus,omitempty"`
	LastRunMessage     string `json:"lastRunMessage,omitempty"`
}

// BackupSettingsUpdateReq carries partial updates from the UI.
type BackupSettingsUpdateReq struct {
	AutoBackupEnabled  bool   `json:"autoBackupEnabled"`
	AutoBackupTime     string `json:"autoBackupTime"`
	AutoBackupDatabase string `json:"autoBackupDatabase"`
	Compression        int    `json:"compression"`
	RetentionDays      int    `json:"retentionDays"`
	MaxBackups         int    `json:"maxBackups"`
	AutoHost           string `json:"autoHost"`
	AutoPort           int    `json:"autoPort"`
	AutoDatabase       string `json:"autoDatabase"`
	AutoUsername       string `json:"autoUsername"`
	AutoPassword       string `json:"autoPassword"`
	ClearAutoPassword  bool   `json:"clearAutoPassword"`
	AutoSSLMode        string `json:"autoSslMode"`
	S3Enabled          bool   `json:"s3Enabled"`
	S3Endpoint         string `json:"s3Endpoint"`
	S3Region           string `json:"s3Region"`
	S3Bucket           string `json:"s3Bucket"`
	S3Prefix           string `json:"s3Prefix"`
	S3AccessKeyID      string `json:"s3AccessKeyId"`
	S3SecretKey        string `json:"s3SecretKey"`
	ClearS3SecretKey   bool   `json:"clearS3SecretKey"`
	S3ForcePathStyle   bool   `json:"s3ForcePathStyle"`
	S3AutoUpload       bool   `json:"s3AutoUpload"`
}

type storedBackupSettings struct {
	BackupSettingsView
	AutoPasswordEnc string `json:"autoPasswordEnc,omitempty"`
	S3SecretKeyEnc  string `json:"s3SecretKeyEnc,omitempty"`
}

// BackupFile represents a stored backup artifact on disk.
type BackupFile struct {
	Filename   string `json:"filename"`
	Database   string `json:"database"`
	Engine     string `json:"engine"`
	Format     string `json:"format"`
	SizeBytes  int64  `json:"sizeBytes"`
	SizePretty string `json:"sizePretty"`
	CreatedAt  string `json:"createdAt"`
}

// BackupResult summarizes a completed backup run.
type BackupResult struct {
	File       BackupFile `json:"file"`
	DurationMs int64      `json:"durationMs"`
	Command    string     `json:"command"`
}

// RestoreResult summarizes a completed restore run.
type RestoreResult struct {
	Filename   string `json:"filename"`
	Database   string `json:"database"`
	DurationMs int64  `json:"durationMs"`
	Output     string `json:"output"`
}

// Manager executes zero-conflict backups, restores, retention pruning, and optional S3 sync.
type Manager struct {
	mu           sync.RWMutex
	cfg          *config.Config
	encKey       [32]byte
	settings     storedBackupSettings
	settingsPath string
	lastRunDate  string
}

// NewManager creates a backup manager and derives/loads the local encryption key.
func NewManager(cfg *config.Config) *Manager {
	var key [32]byte
	if cfg != nil && (cfg.DataDir != "" || cfg.ProfileEncKey != "") {
		if k, err := cryptoutil.LoadOrCreateKey(cfg.DataDir, cfg.ProfileEncKey); err == nil {
			key = k
		} else {
			log.Printf("warning: backup settings encryption key fallback: %v", err)
		}
	}
	return NewManagerWithKey(cfg, key)
}

// NewManagerWithKey creates a backup manager with an explicit 32-byte encryption key.
func NewManagerWithKey(cfg *config.Config, key [32]byte) *Manager {
	m := &Manager{
		cfg:      cfg,
		encKey:   key,
		settings: seedSettingsFromConfig(cfg, key),
	}
	if cfg != nil && cfg.DataDir != "" {
		m.settingsPath = filepath.Join(cfg.DataDir, "backup_settings.json")
		var persisted storedBackupSettings
		if err := store.ReadJSONFile(m.settingsPath, &persisted); err == nil {
			if persisted.AutoBackupTime == "" {
				persisted.AutoBackupTime = m.settings.AutoBackupTime
			}
			if persisted.Compression == 0 && m.settings.Compression > 0 {
				persisted.Compression = m.settings.Compression
			}
			persisted.HasAutoPassword = persisted.AutoPasswordEnc != ""
			persisted.HasS3SecretKey = persisted.S3SecretKeyEnc != ""
			m.settings = persisted
		}
	}
	return m
}

func seedSettingsFromConfig(cfg *config.Config, key [32]byte) storedBackupSettings {
	s := storedBackupSettings{
		BackupSettingsView: BackupSettingsView{
			AutoBackupTime: "02:00",
			Compression:    6,
			AutoPort:       5432,
			AutoSSLMode:    "disable",
			S3Region:       "auto",
		},
	}
	if cfg == nil {
		return s
	}
	s.AutoBackupEnabled = cfg.AutoBackupEnabled
	if cfg.AutoBackupTime != "" {
		s.AutoBackupTime = cfg.AutoBackupTime
	}
	s.RetentionDays = cfg.AutoBackupRetentionDays
	if cfg.AutoBackupDSN != "" {
		if u, err := url.Parse(cfg.AutoBackupDSN); err == nil {
			s.AutoHost = u.Hostname()
			if port, _ := strconv.Atoi(u.Port()); port > 0 {
				s.AutoPort = port
			}
			if u.User != nil {
				s.AutoUsername = u.User.Username()
				if pass, ok := u.User.Password(); ok && pass != "" {
					if enc, err := cryptoutil.Encrypt(key, pass); err == nil {
						s.AutoPasswordEnc = enc
						s.HasAutoPassword = true
					}
				}
			}
			db := strings.TrimPrefix(u.Path, "/")
			if db == "" {
				db = "postgres"
			}
			s.AutoDatabase = db
			if ssl := u.Query().Get("sslmode"); ssl != "" {
				s.AutoSSLMode = ssl
			}
		}
	}
	return s
}

// GetSettings returns the current backup schedule, retention, and S3 sync settings without secrets.
func (m *Manager) GetSettings() BackupSettingsView {
	m.mu.RLock()
	defer m.mu.RUnlock()
	view := m.settings.BackupSettingsView
	view.HasAutoPassword = m.settings.AutoPasswordEnc != ""
	view.HasS3SecretKey = m.settings.S3SecretKeyEnc != ""
	return view
}

// UpdateSettings validates, persists, and returns updated backup & S3 settings.
func (m *Manager) UpdateSettings(req BackupSettingsUpdateReq) (BackupSettingsView, error) {
	hhmm := strings.TrimSpace(req.AutoBackupTime)
	if hhmm == "" {
		hhmm = "02:00"
	}
	if !hhmmRe.MatchString(hhmm) {
		return BackupSettingsView{}, fmt.Errorf("%w: autoBackupTime must be in HH:MM 24-hour format", engine.ErrInvalidInput)
	}
	comp := req.Compression
	if comp < 0 || comp > 9 {
		comp = 6
	}
	retDays := req.RetentionDays
	if retDays < 0 {
		retDays = 0
	}
	maxBackups := req.MaxBackups
	if maxBackups < 0 {
		maxBackups = 0
	}
	port := req.AutoPort
	if port <= 0 || port > 65535 {
		port = 5432
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.settings
	s.AutoBackupEnabled = req.AutoBackupEnabled
	s.AutoBackupTime = hhmm
	s.AutoBackupDatabase = strings.TrimSpace(req.AutoBackupDatabase)
	s.Compression = comp
	s.RetentionDays = retDays
	s.MaxBackups = maxBackups
	s.AutoHost = strings.TrimSpace(req.AutoHost)
	s.AutoPort = port
	s.AutoDatabase = strings.TrimSpace(req.AutoDatabase)
	s.AutoUsername = strings.TrimSpace(req.AutoUsername)
	s.AutoSSLMode = strings.TrimSpace(req.AutoSSLMode)
	if s.AutoSSLMode == "" {
		s.AutoSSLMode = "disable"
	}
	if req.ClearAutoPassword {
		s.AutoPasswordEnc = ""
	} else if req.AutoPassword != "" {
		enc, err := cryptoutil.Encrypt(m.encKey, req.AutoPassword)
		if err != nil {
			return BackupSettingsView{}, err
		}
		s.AutoPasswordEnc = enc
	}

	s.S3Enabled = req.S3Enabled
	s.S3Endpoint = strings.TrimSpace(req.S3Endpoint)
	s.S3Region = strings.TrimSpace(req.S3Region)
	if s.S3Region == "" {
		s.S3Region = "auto"
	}
	s.S3Bucket = strings.TrimSpace(req.S3Bucket)
	s.S3Prefix = strings.Trim(strings.TrimSpace(req.S3Prefix), "/")
	s.S3AccessKeyID = strings.TrimSpace(req.S3AccessKeyID)
	if req.ClearS3SecretKey {
		s.S3SecretKeyEnc = ""
	} else if req.S3SecretKey != "" {
		enc, err := cryptoutil.Encrypt(m.encKey, req.S3SecretKey)
		if err != nil {
			return BackupSettingsView{}, err
		}
		s.S3SecretKeyEnc = enc
	}
	s.S3ForcePathStyle = req.S3ForcePathStyle
	s.S3AutoUpload = req.S3AutoUpload
	s.HasAutoPassword = s.AutoPasswordEnc != ""
	s.HasS3SecretKey = s.S3SecretKeyEnc != ""

	if m.settingsPath != "" {
		if err := store.WriteJSONAtomic(m.settingsPath, s, 0o600); err != nil {
			return BackupSettingsView{}, err
		}
	}
	m.settings = s
	return s.BackupSettingsView, nil
}

// Capabilities checks availability and versions of pg_dump, pg_restore, psql, mysqldump, mysql, and S3.
func (m *Manager) Capabilities(ctx context.Context) ToolCapabilities {
	m.mu.RLock()
	s3Ready := m.settings.S3Enabled && m.settings.S3Bucket != "" && m.settings.S3AccessKeyID != "" && m.settings.S3SecretKeyEnc != ""
	m.mu.RUnlock()

	psqlBin := m.cfg.PsqlPath
	if psqlBin == "" {
		psqlBin = "psql"
	}
	mysqlBin := m.cfg.MySQLPath
	if mysqlBin == "" {
		mysqlBin = "mysql"
	}
	caps := ToolCapabilities{
		BackupDir:    m.cfg.BackupDir,
		S3Configured: s3Ready,
	}
	if ver, ok := probeBinary(ctx, m.cfg.PgDumpPath, "--version"); ok {
		caps.PgDumpAvailable = true
		caps.PgDumpVersion = ver
	}
	if ver, ok := probeBinary(ctx, m.cfg.PgRestorePath, "--version"); ok {
		caps.PgRestoreAvailable = true
		caps.PgRestoreVersion = ver
	}
	if _, ok := probeBinary(ctx, psqlBin, "--version"); ok {
		caps.PsqlAvailable = true
	}
	if _, ok := probeBinary(ctx, m.cfg.MySQLDumpPath, "--version"); ok {
		caps.MySQLDumpAvailable = true
	}
	if _, ok := probeBinary(ctx, mysqlBin, "--version"); ok {
		caps.MySQLClientAvailable = true
	}
	return caps
}

func probeBinary(ctx context.Context, bin, flag string) (string, bool) {
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return "", false
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, resolved, flag).Output()
	if err != nil {
		return resolved, true
	}
	return strings.TrimSpace(string(out)), true
}

// ListBackups enumerates backup files in BackupDir sorted newest first.
func (m *Manager) ListBackups() ([]BackupFile, error) {
	entries, err := os.ReadDir(m.cfg.BackupDir)
	if err != nil {
		return nil, err
	}

	type timedFile struct {
		file BackupFile
		mod  time.Time
	}
	var items []timedFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".tmp") || strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(name, ".dump") && !strings.HasSuffix(name, ".sql") && !strings.HasSuffix(name, ".sqlite.bak") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		dbName, eng, format := parseBackupMeta(name)
		items = append(items, timedFile{
			file: BackupFile{
				Filename:   name,
				Database:   dbName,
				Engine:     eng,
				Format:     format,
				SizeBytes:  info.Size(),
				SizePretty: engine.HumanBytes(info.Size()),
				CreatedAt:  info.ModTime().UTC().Format(time.RFC3339),
			},
			mod: info.ModTime(),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].mod.After(items[j].mod)
	})

	out := make([]BackupFile, 0, len(items))
	for _, it := range items {
		out = append(out, it.file)
	}
	return out, nil
}

func parseBackupMeta(filename string) (dbName, eng, format string) {
	eng = "postgres"
	format = "custom-binary"
	base := filename
	switch {
	case strings.HasSuffix(filename, ".dump"):
		base = strings.TrimSuffix(filename, ".dump")
	case strings.HasSuffix(filename, ".sql"):
		base = strings.TrimSuffix(filename, ".sql")
		eng = "mysql"
		format = "sql-snapshot"
	case strings.HasSuffix(filename, ".sqlite.bak"):
		base = strings.TrimSuffix(filename, ".sqlite.bak")
		eng = "sqlite"
		format = "sqlite-snapshot"
	}

	// Strip trailing _YYYY_MM_DD_HHMMSS timestamp if present
	parts := strings.Split(base, "_")
	if len(parts) > 4 {
		last := parts[len(parts)-1]
		if len(last) == 6 {
			dbName = strings.Join(parts[:len(parts)-4], "_")
		}
	}
	if dbName == "" {
		dbName = base
	}
	return dbName, eng, format
}

// CreateBackup produces a zero-conflict backup snapshot for the target database.
func (m *Manager) CreateBackup(ctx context.Context, drv engine.Driver, dbName string, compression int) (*BackupResult, error) {
	if compression < 0 || compression > 9 {
		compression = 6
	}
	if strings.TrimSpace(dbName) == "" {
		dbName = drv.Config().Database
	}
	if strings.HasPrefix(dbName, "-") || strings.ContainsAny(dbName, "/\\:\x00") {
		return nil, fmt.Errorf("%w: invalid database name for backup", engine.ErrInvalidInput)
	}
	if err := os.MkdirAll(m.cfg.BackupDir, 0o750); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}

	started := time.Now()
	ts := started.Format("2006_01_02_150405")
	sanitizedDB := sanitizeDBForFilename(dbName)

	var (
		res *BackupResult
		err error
	)
	switch d := drv.(type) {
	case *postgres.Driver:
		res, err = m.backupPostgres(ctx, d.RawConfig(), dbName, sanitizedDB, ts, compression, started)
	case *mysql.Driver:
		res, err = m.backupMySQL(ctx, d.RawConfig(), dbName, sanitizedDB, ts, started)
	case *sqlite.Driver:
		res, err = m.backupSQLite(ctx, d, sanitizedDB, ts, started)
	default:
		return nil, engine.ErrUnsupported
	}
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	autoUpload := m.settings.S3Enabled && m.settings.S3AutoUpload
	m.mu.RUnlock()
	if autoUpload {
		if _, s3Err := m.PushBackupToS3(ctx, res.File.Filename); s3Err != nil {
			res.Command += fmt.Sprintf(" | S3 upload failed: %v", s3Err)
		} else {
			res.Command += " | S3 auto-uploaded"
		}
	}
	return res, nil
}

func (m *Manager) backupPostgres(
	ctx context.Context,
	cfg engine.ConnectionConfig,
	dbName, sanitizedDB, ts string,
	compression int,
	started time.Time,
) (*BackupResult, error) {
	binPath, err := exec.LookPath(m.cfg.PgDumpPath)
	if err != nil {
		return nil, fmt.Errorf("pg_dump not found on this machine (%s). Install postgresql-client locally or run on the database VPS", m.cfg.PgDumpPath)
	}

	filename := fmt.Sprintf("%s_%s.dump", sanitizedDB, ts)
	finalPath := filepath.Join(m.cfg.BackupDir, filename)
	tmpPath := finalPath + ".tmp"

	args := []string{
		"-Fc",
		"-Z", strconv.Itoa(compression),
		"--no-password",
		"-h", cfg.Host,
		"-p", strconv.Itoa(cfg.Port),
		"-U", cfg.Username,
		"-d", dbName,
		"-f", tmpPath,
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+cfg.Password)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpPath)
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("pg_dump failed: %s", msg)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}

	fi, err := os.Stat(finalPath)
	if err != nil {
		return nil, err
	}

	return &BackupResult{
		File: BackupFile{
			Filename:   filename,
			Database:   dbName,
			Engine:     "postgres",
			Format:     "custom-binary",
			SizeBytes:  fi.Size(),
			SizePretty: engine.HumanBytes(fi.Size()),
			CreatedAt:  fi.ModTime().UTC().Format(time.RFC3339),
		},
		DurationMs: time.Since(started).Milliseconds(),
		Command:    fmt.Sprintf("pg_dump -Fc -Z %d -d %s -f %s", compression, dbName, filename),
	}, nil
}

func (m *Manager) backupMySQL(
	ctx context.Context,
	cfg engine.ConnectionConfig,
	dbName, sanitizedDB, ts string,
	started time.Time,
) (*BackupResult, error) {
	binPath, err := exec.LookPath(m.cfg.MySQLDumpPath)
	if err != nil {
		return nil, fmt.Errorf("mysqldump not found on this machine (%s)", m.cfg.MySQLDumpPath)
	}
	filename := fmt.Sprintf("%s_%s.sql", sanitizedDB, ts)
	finalPath := filepath.Join(m.cfg.BackupDir, filename)
	tmpPath := finalPath + ".tmp"

	outFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--single-transaction",
		"--routines",
		"--triggers",
		"-h", cfg.Host,
		"-P", strconv.Itoa(cfg.Port),
		"-u", cfg.Username,
		dbName,
	}
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.Password)
	cmd.Stdout = outFile
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	_ = outFile.Close()
	if runErr != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("mysqldump failed: %s", strings.TrimSpace(stderr.String()))
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	fi, err := os.Stat(finalPath)
	if err != nil {
		return nil, err
	}
	return &BackupResult{
		File: BackupFile{
			Filename:   filename,
			Database:   dbName,
			Engine:     "mysql",
			Format:     "sql-snapshot",
			SizeBytes:  fi.Size(),
			SizePretty: engine.HumanBytes(fi.Size()),
			CreatedAt:  fi.ModTime().UTC().Format(time.RFC3339),
		},
		DurationMs: time.Since(started).Milliseconds(),
		Command:    fmt.Sprintf("mysqldump --single-transaction %s > %s", dbName, filename),
	}, nil
}

func (m *Manager) backupSQLite(ctx context.Context, d *sqlite.Driver, sanitizedDB, ts string, started time.Time) (*BackupResult, error) {
	filename := fmt.Sprintf("%s_%s.sqlite.bak", sanitizedDB, ts)
	finalPath := filepath.Join(m.cfg.BackupDir, filename)
	escapedPath := strings.ReplaceAll(finalPath, "'", "''")
	if _, err := d.ExecuteSQL(ctx, "", fmt.Sprintf("VACUUM INTO '%s'", escapedPath)); err != nil {
		return nil, fmt.Errorf("sqlite VACUUM INTO failed: %w", err)
	}
	fi, err := os.Stat(finalPath)
	if err != nil {
		return nil, err
	}
	return &BackupResult{
		File: BackupFile{
			Filename:   filename,
			Database:   d.Config().Database,
			Engine:     "sqlite",
			Format:     "sqlite-snapshot",
			SizeBytes:  fi.Size(),
			SizePretty: engine.HumanBytes(fi.Size()),
			CreatedAt:  fi.ModTime().UTC().Format(time.RFC3339),
		},
		DurationMs: time.Since(started).Milliseconds(),
		Command:    fmt.Sprintf("VACUUM INTO '%s'", filename),
	}, nil
}

// SaveUploadedBackup validates and writes an uploaded backup file into BackupDir.
func (m *Manager) SaveUploadedBackup(filename string, r io.Reader) (*BackupFile, error) {
	base := filepath.Base(strings.TrimSpace(filename))
	if !safeFilenameRe.MatchString(base) || strings.Contains(base, "..") {
		return nil, fmt.Errorf("%w: invalid backup filename (use alphanumeric, _, ., -)", engine.ErrInvalidInput)
	}
	if !strings.HasSuffix(base, ".dump") && !strings.HasSuffix(base, ".sql") && !strings.HasSuffix(base, ".sqlite.bak") {
		return nil, fmt.Errorf("%w: only .dump, .sql, or .sqlite.bak files are accepted", engine.ErrInvalidInput)
	}
	if err := os.MkdirAll(m.cfg.BackupDir, 0o750); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}

	destPath := filepath.Join(m.cfg.BackupDir, base)
	tmpPath := destPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return nil, err
	}
	written, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil, closeErr
	}
	if written == 0 {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("%w: uploaded backup file is empty", engine.ErrInvalidInput)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}

	fi, err := os.Stat(destPath)
	if err != nil {
		return nil, err
	}
	dbName, eng, format := parseBackupMeta(base)
	return &BackupFile{
		Filename:   base,
		Database:   dbName,
		Engine:     eng,
		Format:     format,
		SizeBytes:  fi.Size(),
		SizePretty: engine.HumanBytes(fi.Size()),
		CreatedAt:  fi.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

func ensureTargetDB(ctx context.Context, drv engine.Driver, targetDB string) error {
	dbs, err := drv.ListDatabases(ctx)
	if err != nil {
		return nil
	}
	for _, db := range dbs {
		if db.Name == targetDB {
			return nil
		}
	}
	if err := drv.CreateDatabase(ctx, engine.CreateDatabaseReq{Name: targetDB, Encoding: "UTF8"}); err != nil {
		return fmt.Errorf("create target database %q: %w", targetDB, err)
	}
	return nil
}

// RestoreBackup runs pg_restore (.dump), psql (.sql), or mysql (.sql) restore into targetDB.
func (m *Manager) RestoreBackup(
	ctx context.Context,
	drv engine.Driver,
	filename, targetDB string,
	jobs int,
	cleanFirst, createIfMissing bool,
) (*RestoreResult, error) {
	filePath, err := m.ResolveBackupPath(filename)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(targetDB) == "" || strings.HasPrefix(targetDB, "-") || strings.ContainsAny(targetDB, "/\\:\x00") {
		return nil, fmt.Errorf("%w: invalid target database", engine.ErrInvalidInput)
	}
	if jobs < 1 {
		jobs = 4
	}
	if jobs > 16 {
		jobs = 16
	}

	started := time.Now()
	switch d := drv.(type) {
	case *postgres.Driver:
		if createIfMissing {
			if err := ensureTargetDB(ctx, drv, targetDB); err != nil {
				return nil, err
			}
		}
		if strings.HasSuffix(filename, ".dump") {
			return m.restorePostgresDump(ctx, d.RawConfig(), filePath, filename, targetDB, jobs, cleanFirst, started)
		}
		if strings.HasSuffix(filename, ".sql") {
			return m.restorePostgresSQL(ctx, d.RawConfig(), filePath, filename, targetDB, started)
		}
		return nil, fmt.Errorf("%w: PostgreSQL supports .dump (pg_restore) or .sql (psql) backups", engine.ErrInvalidInput)

	case *mysql.Driver:
		if !strings.HasSuffix(filename, ".sql") {
			return nil, fmt.Errorf("%w: MySQL restore requires a .sql backup file", engine.ErrInvalidInput)
		}
		if createIfMissing {
			if err := ensureTargetDB(ctx, drv, targetDB); err != nil {
				return nil, err
			}
		}
		return m.restoreMySQL(ctx, d.RawConfig(), filePath, filename, targetDB, started)

	default:
		return nil, fmt.Errorf("%w: automated restore supports PostgreSQL (.dump/.sql) and MySQL (.sql) backups", engine.ErrUnsupported)
	}
}

func (m *Manager) restorePostgresDump(
	ctx context.Context,
	cfg engine.ConnectionConfig,
	filePath, filename, targetDB string,
	jobs int,
	cleanFirst bool,
	started time.Time,
) (*RestoreResult, error) {
	binPath, err := exec.LookPath(m.cfg.PgRestorePath)
	if err != nil {
		return nil, fmt.Errorf("pg_restore not found on this machine (%s)", m.cfg.PgRestorePath)
	}
	args := []string{
		"-j", strconv.Itoa(jobs),
		"--no-password",
		"--no-owner",
		"-h", cfg.Host,
		"-p", strconv.Itoa(cfg.Port),
		"-U", cfg.Username,
		"-d", targetDB,
	}
	if cleanFirst {
		args = append(args, "--clean", "--if-exists")
	}
	args = append(args, filePath)

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+cfg.Password)
	if cfg.SSLMode != "" {
		cmd.Env = append(cmd.Env, "PGSSLMODE="+cfg.SSLMode)
	}
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pg_restore failed: %s (%v)", strings.TrimSpace(outBuf.String()), err)
	}
	output := strings.TrimSpace(outBuf.String())
	if output == "" {
		output = fmt.Sprintf("Restored %s into %s successfully using %d parallel workers.", filename, targetDB, jobs)
	}
	return &RestoreResult{
		Filename:   filename,
		Database:   targetDB,
		DurationMs: time.Since(started).Milliseconds(),
		Output:     output,
	}, nil
}

func (m *Manager) restorePostgresSQL(
	ctx context.Context,
	cfg engine.ConnectionConfig,
	filePath, filename, targetDB string,
	started time.Time,
) (*RestoreResult, error) {
	psqlBin := m.cfg.PsqlPath
	if psqlBin == "" {
		psqlBin = "psql"
	}
	binPath, err := exec.LookPath(psqlBin)
	if err != nil {
		return nil, fmt.Errorf("psql client not found on this machine (%s)", psqlBin)
	}
	args := []string{
		"-v", "ON_ERROR_STOP=1",
		"--no-password",
		"-h", cfg.Host,
		"-p", strconv.Itoa(cfg.Port),
		"-U", cfg.Username,
		"-d", targetDB,
		"-f", filePath,
	}
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+cfg.Password)
	if cfg.SSLMode != "" {
		cmd.Env = append(cmd.Env, "PGSSLMODE="+cfg.SSLMode)
	}
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("psql restore failed: %s (%v)", strings.TrimSpace(outBuf.String()), err)
	}
	output := strings.TrimSpace(outBuf.String())
	if output == "" {
		output = fmt.Sprintf("Executed SQL backup %s into %s via psql.", filename, targetDB)
	}
	return &RestoreResult{
		Filename:   filename,
		Database:   targetDB,
		DurationMs: time.Since(started).Milliseconds(),
		Output:     output,
	}, nil
}

func (m *Manager) restoreMySQL(
	ctx context.Context,
	cfg engine.ConnectionConfig,
	filePath, filename, targetDB string,
	started time.Time,
) (*RestoreResult, error) {
	mysqlBin := m.cfg.MySQLPath
	if mysqlBin == "" {
		mysqlBin = "mysql"
	}
	binPath, err := exec.LookPath(mysqlBin)
	if err != nil {
		return nil, fmt.Errorf("mysql client CLI not found on this machine (%s)", mysqlBin)
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	args := []string{
		"-h", cfg.Host,
		"-P", strconv.Itoa(cfg.Port),
		"-u", cfg.Username,
		targetDB,
	}
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+cfg.Password)
	cmd.Stdin = f
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mysql restore failed: %s (%v)", strings.TrimSpace(outBuf.String()), err)
	}
	output := strings.TrimSpace(outBuf.String())
	if output == "" {
		output = fmt.Sprintf("Restored MySQL SQL backup %s into %s.", filename, targetDB)
	}
	return &RestoreResult{
		Filename:   filename,
		Database:   targetDB,
		DurationMs: time.Since(started).Milliseconds(),
		Output:     output,
	}, nil
}

// ResolveBackupPath safely resolves a backup filename strictly within BackupDir.
func (m *Manager) ResolveBackupPath(filename string) (string, error) {
	if !safeFilenameRe.MatchString(filename) || strings.Contains(filename, "..") {
		return "", fmt.Errorf("%w: invalid backup filename", engine.ErrInvalidInput)
	}
	base := filepath.Base(filename)
	if base != filename {
		return "", fmt.Errorf("%w: invalid backup filename", engine.ErrInvalidInput)
	}
	full := filepath.Join(m.cfg.BackupDir, base)
	if _, err := os.Stat(full); err != nil {
		return "", err
	}
	return full, nil
}

// DeleteBackup removes a backup file from BackupDir.
func (m *Manager) DeleteBackup(filename string) error {
	full, err := m.ResolveBackupPath(filename)
	if err != nil {
		return err
	}
	return os.Remove(full)
}

// OpenBackup opens a backup file for streaming download.
func (m *Manager) OpenBackup(filename string) (io.ReadCloser, int64, error) {
	full, err := m.ResolveBackupPath(filename)
	if err != nil {
		return nil, 0, err
	}
	fi, err := os.Stat(full)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, 0, err
	}
	return f, fi.Size(), nil
}

// StartScheduler monitors dynamic auto-backup settings and executes daily scheduled backups.
func (m *Manager) StartScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				hhmm := now.Format("15:04")
				today := now.Format("2006-01-02")

				m.mu.Lock()
				shouldRun := m.settings.AutoBackupEnabled && hhmm == m.settings.AutoBackupTime && m.lastRunDate != today
				if shouldRun {
					m.lastRunDate = today
				}
				m.mu.Unlock()

				if shouldRun {
					if _, err := m.RunScheduledNow(ctx, nil); err != nil {
						log.Printf("auto-backup scheduler run failed: %v", err)
					}
				}
			}
		}
	}()
}

// RunScheduledNow executes the configured scheduled backup immediately and applies retention pruning.
func (m *Manager) RunScheduledNow(ctx context.Context, optDrv engine.Driver) ([]BackupResult, error) {
	m.mu.RLock()
	s := m.settings
	m.mu.RUnlock()

	var (
		drv      engine.Driver
		ownDrv   bool
		comp     = s.Compression
		targetDB = strings.TrimSpace(s.AutoBackupDatabase)
	)
	if comp < 0 || comp > 9 {
		comp = 6
	}

	if s.AutoHost != "" || s.AutoUsername != "" {
		pass, err := cryptoutil.Decrypt(m.encKey, s.AutoPasswordEnc)
		if err != nil {
			m.recordScheduleStatus("error", fmt.Sprintf("decrypt scheduled password: %v", err))
			return nil, fmt.Errorf("decrypt scheduled password: %w", err)
		}
		port := s.AutoPort
		if port <= 0 {
			port = 5432
		}
		db := s.AutoDatabase
		if db == "" {
			db = "postgres"
		}
		ssl := s.AutoSSLMode
		if ssl == "" {
			ssl = "disable"
		}
		host := s.AutoHost
		if host == "" {
			host = "127.0.0.1"
		}
		pgDrv, err := postgres.New(ctx, engine.ConnectionConfig{
			Engine:   engine.EnginePostgres,
			Host:     host,
			Port:     port,
			Database: db,
			Username: s.AutoUsername,
			Password: pass,
			SSLMode:  ssl,
		})
		if err != nil {
			m.recordScheduleStatus("error", err.Error())
			return nil, fmt.Errorf("scheduled backup connect failed: %w", err)
		}
		drv = pgDrv
		ownDrv = true
	} else if optDrv != nil {
		drv = optDrv
	} else {
		msg := "no scheduled PostgreSQL credentials configured"
		m.recordScheduleStatus("error", msg)
		return nil, fmt.Errorf("%w: %s", engine.ErrInvalidInput, msg)
	}
	if ownDrv {
		defer drv.Close()
	}

	var targets []string
	if targetDB != "" && targetDB != "*" && !strings.EqualFold(targetDB, "all") {
		for _, part := range strings.Split(targetDB, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				targets = append(targets, trimmed)
			}
		}
	} else {
		dbs, err := drv.ListDatabases(ctx)
		if err != nil {
			m.recordScheduleStatus("error", err.Error())
			return nil, err
		}
		for _, item := range dbs {
			if item.AllowConnect {
				targets = append(targets, item.Name)
			}
		}
	}

	var (
		results []BackupResult
		errs    []string
	)
	for _, dbName := range targets {
		res, err := m.CreateBackup(ctx, drv, dbName, comp)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", dbName, err))
			log.Printf("auto-backup failed for %s: %v", dbName, err)
			continue
		}
		results = append(results, *res)
		log.Printf("auto-backup complete: %s (%s, %dms)", res.File.Filename, res.File.SizePretty, res.DurationMs)
	}

	pruned, _ := m.PruneBackups()
	if len(errs) > 0 && len(results) == 0 {
		msg := strings.Join(errs, "; ")
		m.recordScheduleStatus("error", msg)
		return nil, errors.New(msg)
	}
	status := "ok"
	msg := fmt.Sprintf("Backed up %d database(s), pruned %d old file(s).", len(results), pruned)
	if len(errs) > 0 {
		status = "partial"
		msg += " Warnings: " + strings.Join(errs, "; ")
	}
	m.recordScheduleStatus(status, msg)
	return results, nil
}

func (m *Manager) recordScheduleStatus(status, msg string) {
	m.mu.Lock()
	m.settings.LastRunAt = time.Now().UTC().Format(time.RFC3339)
	m.settings.LastRunStatus = status
	m.settings.LastRunMessage = msg
	snapshot := m.settings
	path := m.settingsPath
	m.mu.Unlock()

	if path != "" {
		_ = store.WriteJSONAtomic(path, snapshot, 0o600)
	}
}

// PruneBackups deletes old backups exceeding RetentionDays or MaxBackups count and returns removed count.
func (m *Manager) PruneBackups() (int, error) {
	m.mu.RLock()
	retDays := m.settings.RetentionDays
	maxBackups := m.settings.MaxBackups
	m.mu.RUnlock()

	if retDays <= 0 && maxBackups <= 0 {
		return 0, nil
	}

	items, err := m.ListBackups()
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-time.Duration(retDays) * 24 * time.Hour)
	deleted := 0
	kept := 0

	for _, item := range items {
		full := filepath.Join(m.cfg.BackupDir, item.Filename)
		createdAt, parseErr := time.Parse(time.RFC3339, item.CreatedAt)
		tooOld := retDays > 0 && parseErr == nil && createdAt.Before(cutoff)
		overCap := maxBackups > 0 && kept >= maxBackups

		if tooOld || overCap {
			if os.Remove(full) == nil {
				deleted++
			}
			continue
		}
		kept++
	}
	return deleted, nil
}

func (m *Manager) buildS3Client() (*s3Client, error) {
	m.mu.RLock()
	s := m.settings
	m.mu.RUnlock()

	secret, err := cryptoutil.Decrypt(m.encKey, s.S3SecretKeyEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt S3 secret key: %w", err)
	}
	return newS3Client(S3Config{
		Enabled:         s.S3Enabled,
		Endpoint:        s.S3Endpoint,
		Region:          s.S3Region,
		Bucket:          s.S3Bucket,
		Prefix:          s.S3Prefix,
		AccessKeyID:     s.S3AccessKeyID,
		SecretAccessKey: secret,
		ForcePathStyle:  s.S3ForcePathStyle,
		AutoUpload:      s.S3AutoUpload,
	})
}

// TestS3 verifies S3/R2 connectivity by listing at most 1 key from the configured bucket.
func (m *Manager) TestS3(ctx context.Context) error {
	c, err := m.buildS3Client()
	if err != nil {
		return err
	}
	_, err = c.listObjects(ctx, "1")
	return err
}

// ListS3Backups returns remote backup artifacts stored in the configured S3/R2 bucket.
func (m *Manager) ListS3Backups(ctx context.Context) ([]S3Object, error) {
	c, err := m.buildS3Client()
	if err != nil {
		return nil, err
	}
	return c.listObjects(ctx, "200")
}

// PushBackupToS3 uploads a local backup file to the configured S3/R2 bucket.
func (m *Manager) PushBackupToS3(ctx context.Context, filename string) (S3Object, error) {
	c, err := m.buildS3Client()
	if err != nil {
		return S3Object{}, err
	}
	localPath, err := m.ResolveBackupPath(filename)
	if err != nil {
		return S3Object{}, err
	}
	fi, err := os.Stat(localPath)
	if err != nil {
		return S3Object{}, err
	}
	key := c.objectKey(filepath.Base(localPath))
	if err := c.uploadFile(ctx, localPath, key); err != nil {
		return S3Object{}, err
	}
	return S3Object{
		Key:          key,
		Filename:     filepath.Base(localPath),
		SizeBytes:    fi.Size(),
		SizePretty:   engine.HumanBytes(fi.Size()),
		LastModified: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// PullBackupFromS3 downloads a remote backup object from S3/R2 into the local BackupDir.
func (m *Manager) PullBackupFromS3(ctx context.Context, key string) (*BackupFile, error) {
	c, err := m.buildS3Client()
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || strings.Contains(trimmed, "..") {
		return nil, fmt.Errorf("%w: invalid S3 object key", engine.ErrInvalidInput)
	}
	base := path.Base(trimmed)
	if !safeFilenameRe.MatchString(base) ||
		(!strings.HasSuffix(base, ".dump") && !strings.HasSuffix(base, ".sql") && !strings.HasSuffix(base, ".sqlite.bak")) {
		return nil, fmt.Errorf("%w: remote filename must be a valid .dump, .sql, or .sqlite.bak file", engine.ErrInvalidInput)
	}
	dest := filepath.Join(m.cfg.BackupDir, base)
	if err := c.downloadFile(ctx, trimmed, dest); err != nil {
		return nil, err
	}
	fi, err := os.Stat(dest)
	if err != nil {
		return nil, err
	}
	dbName, eng, format := parseBackupMeta(base)
	return &BackupFile{
		Filename:   base,
		Database:   dbName,
		Engine:     eng,
		Format:     format,
		SizeBytes:  fi.Size(),
		SizePretty: engine.HumanBytes(fi.Size()),
		CreatedAt:  fi.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// DeleteS3Backup deletes a remote object from S3/R2.
func (m *Manager) DeleteS3Backup(ctx context.Context, key string) error {
	c, err := m.buildS3Client()
	if err != nil {
		return err
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%w: S3 key is required", engine.ErrInvalidInput)
	}
	return c.deleteObject(ctx, strings.TrimSpace(key))
}

func sanitizeDBForFilename(dbName string) string {
	var b strings.Builder
	for _, r := range dbName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "database"
	}
	return out
}

package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds runtime settings loaded from .env and process environment.
type Config struct {
	AppAddr                 string
	StaticDir               string
	BackupDir               string
	DataDir                 string
	ProfileEncKey           string
	SessionTTL              time.Duration
	CookieSecure            bool
	DefaultEngine           string
	DefaultHost             string
	DefaultPort             int
	DefaultDatabase         string
	DefaultUsername         string
	DefaultSSLMode          string
	PgDumpPath              string
	PgRestorePath           string
	PsqlPath                string
	MySQLDumpPath           string
	MySQLPath               string
	AutoBackupEnabled       bool
	AutoBackupTime          string
	AutoBackupDSN           string
	AutoBackupRetentionDays int
}

// Load reads .env (if present) and returns validated configuration defaults.
func Load() (*Config, error) {
	_ = godotenv.Load()

	ttlHours := envInt("SESSION_TTL_HOURS", 12)
	if ttlHours < 1 {
		ttlHours = 12
	}

	cfg := &Config{
		AppAddr:                 envStr("APP_ADDR", ":8080"),
		StaticDir:               envStr("STATIC_DIR", "./web"),
		BackupDir:               envStr("BACKUP_DIR", "./backups"),
		DataDir:                 envStr("DATA_DIR", "./data"),
		ProfileEncKey:           strings.TrimSpace(os.Getenv("PROFILE_ENCRYPTION_KEY")),
		SessionTTL:              time.Duration(ttlHours) * time.Hour,
		CookieSecure:            envBool("COOKIE_SECURE", false),
		DefaultEngine:           envStr("DEFAULT_ENGINE", "postgres"),
		DefaultHost:             envStr("DEFAULT_HOST", "127.0.0.1"),
		DefaultPort:             envInt("DEFAULT_PORT", 5432),
		DefaultDatabase:         envStr("DEFAULT_DATABASE", "postgres"),
		DefaultUsername:         envStr("DEFAULT_USERNAME", "postgres"),
		DefaultSSLMode:          envStr("DEFAULT_SSLMODE", "disable"),
		PgDumpPath:              envStr("PG_DUMP_PATH", "pg_dump"),
		PgRestorePath:           envStr("PG_RESTORE_PATH", "pg_restore"),
		PsqlPath:                envStr("PSQL_PATH", "psql"),
		MySQLDumpPath:           envStr("MYSQL_DUMP_PATH", "mysqldump"),
		MySQLPath:               envStr("MYSQL_PATH", "mysql"),
		AutoBackupEnabled:       envBool("AUTO_BACKUP_ENABLED", false),
		AutoBackupTime:          envStr("AUTO_BACKUP_TIME", "03:00"),
		AutoBackupDSN:           strings.TrimSpace(os.Getenv("AUTO_BACKUP_DSN")),
		AutoBackupRetentionDays: envInt("AUTO_BACKUP_RETENTION_DAYS", 14),
	}

	if absBackup, err := filepath.Abs(cfg.BackupDir); err == nil {
		cfg.BackupDir = absBackup
	}
	if err := os.MkdirAll(cfg.BackupDir, 0o750); err != nil {
		return nil, err
	}
	if absData, err := filepath.Abs(cfg.DataDir); err == nil {
		cfg.DataDir = absData
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}

	return cfg, nil
}

func envStr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1" || v == "yes"
}

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
)

type dbConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Name     string `json:"name"`
	SSLMode  string `json:"sslmode"`
}

func (c dbConfig) dsn() string {
	cfg := mysql.NewConfig()
	cfg.User = c.User
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	cfg.DBName = c.Name
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Collation = "utf8mb4_0900_ai_ci"
	switch c.SSLMode {
	case "required":
		cfg.TLSConfig = "true"
	case "skip-verify":
		cfg.TLSConfig = "skip-verify"
	case "disable":
		cfg.TLSConfig = "false"
	default:
		cfg.TLSConfig = "preferred"
	}
	cfg.Params = map[string]string{"group_concat_max_len": "1048576", "time_zone": "'+00:00'", "sql_mode": "'STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION'"}
	return cfg.FormatDSN()
}

func (a *App) bootstrapPath() string { return filepath.Join(a.dataDir, "bootstrap.json") }

func (a *App) loadBootstrap() (*dbConfig, error) {
	b, err := os.ReadFile(a.bootstrapPath())
	if err != nil {
		return nil, err
	}
	var c dbConfig
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	pw, err := a.decrypt(c.Password)
	if err != nil {
		return nil, err
	}
	c.Password = pw
	return &c, nil
}

func (a *App) saveBootstrap(c dbConfig) error {
	enc, err := a.encrypt(c.Password)
	if err != nil {
		return err
	}
	c.Password = enc
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(a.bootstrapPath(), b, 0o600)
}

func connectDB(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (a *App) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return a.db.ExecContext(ctx, q, args...)
}
func (a *App) row(ctx context.Context, q string, args ...any) *sql.Row {
	return a.db.QueryRowContext(ctx, q, args...)
}
func (a *App) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return a.db.QueryContext(ctx, q, args...)
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS settings (
	  k VARCHAR(64) PRIMARY KEY,
	  v JSON NOT NULL,
	  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)
	) DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS users (
	  id BIGINT AUTO_INCREMENT PRIMARY KEY,
	  username VARCHAR(128) NOT NULL UNIQUE,
	  display_name VARCHAR(255) NOT NULL DEFAULT '',
	  email VARCHAR(255) NOT NULL DEFAULT '',
	  password_hash VARCHAR(255) NULL,
	  role VARCHAR(16) NOT NULL DEFAULT 'viewer',
	  source VARCHAR(16) NOT NULL DEFAULT 'local',
	  disabled TINYINT(1) NOT NULL DEFAULT 0,
	  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	  last_login DATETIME(6) NULL
	) DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS sessions (
	  token_hash CHAR(64) PRIMARY KEY,
	  user_id BIGINT NOT NULL,
	  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	  expires_at DATETIME(6) NOT NULL,
	  ip VARCHAR(64) NOT NULL DEFAULT '',
	  CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	) DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS domains (
	  id BIGINT AUTO_INCREMENT PRIMARY KEY,
	  domain VARCHAR(255) NOT NULL UNIQUE,
	  dkim_enabled TINYINT(1) NOT NULL DEFAULT 0,
	  dkim_private TEXT NOT NULL DEFAULT (''),
	  dkim_public TEXT NOT NULL DEFAULT (''),
	  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
	) DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS messages (
	  id BIGINT AUTO_INCREMENT PRIMARY KEY,
	  exim_id VARCHAR(40) NOT NULL UNIQUE,
	  received_at DATETIME(6) NOT NULL,
	  sender VARCHAR(320) NOT NULL DEFAULT '',
	  sender_domain VARCHAR(255) NOT NULL DEFAULT '',
	  origin_host VARCHAR(255) NOT NULL DEFAULT '',
	  origin_ip VARCHAR(64) NOT NULL DEFAULT '',
	  subject VARCHAR(1000) NOT NULL DEFAULT '',
	  size INT NOT NULL DEFAULT 0,
	  status VARCHAR(16) NOT NULL DEFAULT 'queued',
	  completed_at DATETIME(6) NULL,
	  INDEX messages_received_idx (received_at),
	  INDEX messages_status_idx (status, received_at),
	  INDEX messages_sender_idx (sender_domain, received_at)
	) DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS deliveries (
	  id BIGINT AUTO_INCREMENT PRIMARY KEY,
	  message_id BIGINT NOT NULL,
	  at DATETIME(6) NOT NULL,
	  recipient VARCHAR(320) NOT NULL,
	  status VARCHAR(16) NOT NULL,
	  remote_host VARCHAR(255) NOT NULL DEFAULT '',
	  remote_ip VARCHAR(64) NOT NULL DEFAULT '',
	  detail TEXT NOT NULL DEFAULT (''),
	  INDEX deliveries_message_idx (message_id),
	  INDEX deliveries_recipient_idx (recipient),
	  CONSTRAINT fk_deliveries_message FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
	) DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS ingest_state (
	  file VARCHAR(32) PRIMARY KEY,
	  inode BIGINT NOT NULL DEFAULT 0,
	  pos BIGINT NOT NULL DEFAULT 0
	) DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS nodes (
	  id BIGINT AUTO_INCREMENT PRIMARY KEY,
	  name VARCHAR(80) NOT NULL UNIQUE,
	  token_hash CHAR(64) NOT NULL UNIQUE,
	  disabled TINYINT(1) NOT NULL DEFAULT 0,
	  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	  last_seen DATETIME(6) NULL,
	  last_ip VARCHAR(64) NOT NULL DEFAULT '',
	  version VARCHAR(32) NOT NULL DEFAULT ''
	) DEFAULT CHARSET=utf8mb4`,
	`CREATE TABLE IF NOT EXISTS audit_log (
	  id BIGINT AUTO_INCREMENT PRIMARY KEY,
	  at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
	  username VARCHAR(128) NOT NULL,
	  action VARCHAR(64) NOT NULL,
	  detail TEXT NOT NULL DEFAULT (''),
	  INDEX audit_at_idx (at)
	) DEFAULT CHARSET=utf8mb4`,
}

func (a *App) hasColumn(ctx context.Context, table, col string) bool {
	var n int
	_ = a.row(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, col).Scan(&n)
	return n > 0
}

func (a *App) hasIndex(ctx context.Context, table, idx string) bool {
	var n int
	_ = a.row(ctx, `SELECT count(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?`, table, idx).Scan(&n)
	return n > 0
}

func (a *App) migrate(ctx context.Context) error {
	for _, s := range schema {
		if _, err := a.exec(ctx, s); err != nil {
			return err
		}
	}
	steps := []struct {
		ok  func() bool
		sql string
	}{
		{func() bool { return a.hasColumn(ctx, "messages", "node_id") }, `ALTER TABLE messages ADD COLUMN node_id BIGINT NOT NULL DEFAULT 0 AFTER id`},
		{func() bool { return a.hasIndex(ctx, "messages", "uq_node_exim") }, `ALTER TABLE messages ADD UNIQUE KEY uq_node_exim (node_id, exim_id)`},
		{func() bool { return !a.hasIndex(ctx, "messages", "exim_id") }, `ALTER TABLE messages DROP INDEX exim_id`},
		{func() bool { return a.hasIndex(ctx, "messages", "messages_node_idx") }, `ALTER TABLE messages ADD INDEX messages_node_idx (node_id, received_at)`},
		{func() bool { return a.hasColumn(ctx, "deliveries", "dk") }, `ALTER TABLE deliveries ADD COLUMN dk BINARY(16) NULL, ADD UNIQUE KEY deliveries_dk (dk)`},
	}
	for _, st := range steps {
		if st.ok() {
			continue
		}
		if _, err := a.exec(ctx, st.sql); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) audit(ctx context.Context, user, action, detail string) {
	if !a.ready() {
		return
	}
	_, _ = a.exec(ctx, `INSERT INTO audit_log(username, action, detail) VALUES (?,?,?)`, user, action, detail)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:     "/" + c.Name,
		RawQuery: "sslmode=" + url.QueryEscape(c.SSLMode),
	}
	return u.String()
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

func connectDB(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS settings (
  key text PRIMARY KEY,
  value jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS users (
  id bigserial PRIMARY KEY,
  username text NOT NULL UNIQUE,
  display_name text NOT NULL DEFAULT '',
  email text NOT NULL DEFAULT '',
  password_hash text,
  role text NOT NULL DEFAULT 'viewer',
  source text NOT NULL DEFAULT 'local',
  disabled boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_login timestamptz
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash text PRIMARY KEY,
  user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  ip text NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS themes (
  id bigserial PRIMARY KEY,
  slug text NOT NULL UNIQUE,
  name text NOT NULL,
  builtin boolean NOT NULL DEFAULT false,
  tokens jsonb NOT NULL,
  logo text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS domains (
  id bigserial PRIMARY KEY,
  domain text NOT NULL UNIQUE,
  dkim_enabled boolean NOT NULL DEFAULT false,
  dkim_private text NOT NULL DEFAULT '',
  dkim_public text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS messages (
  id bigserial PRIMARY KEY,
  exim_id text NOT NULL UNIQUE,
  received_at timestamptz NOT NULL,
  sender text NOT NULL DEFAULT '',
  sender_domain text NOT NULL DEFAULT '',
  origin_host text NOT NULL DEFAULT '',
  origin_ip text NOT NULL DEFAULT '',
  subject text NOT NULL DEFAULT '',
  size integer NOT NULL DEFAULT 0,
  status text NOT NULL DEFAULT 'queued',
  completed_at timestamptz
);
CREATE INDEX IF NOT EXISTS messages_received_idx ON messages (received_at DESC);
CREATE INDEX IF NOT EXISTS messages_status_idx ON messages (status, received_at DESC);
CREATE INDEX IF NOT EXISTS messages_sender_idx ON messages (sender_domain, received_at DESC);
CREATE TABLE IF NOT EXISTS deliveries (
  id bigserial PRIMARY KEY,
  message_id bigint NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  at timestamptz NOT NULL,
  recipient text NOT NULL,
  status text NOT NULL,
  remote_host text NOT NULL DEFAULT '',
  remote_ip text NOT NULL DEFAULT '',
  detail text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS deliveries_message_idx ON deliveries (message_id);
CREATE INDEX IF NOT EXISTS deliveries_recipient_idx ON deliveries (recipient);
CREATE TABLE IF NOT EXISTS ingest_state (
  file text PRIMARY KEY,
  inode bigint NOT NULL DEFAULT 0,
  off bigint NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS audit_log (
  id bigserial PRIMARY KEY,
  at timestamptz NOT NULL DEFAULT now(),
  username text NOT NULL,
  action text NOT NULL,
  detail text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS audit_at_idx ON audit_log (at DESC);
`

func (a *App) migrate(ctx context.Context) error {
	if _, err := a.db.Exec(ctx, schema); err != nil {
		return err
	}
	_, _ = a.db.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pg_trgm`)
	_, _ = a.db.Exec(ctx, `CREATE INDEX IF NOT EXISTS messages_subject_trgm ON messages USING gin (subject gin_trgm_ops)`)
	_, _ = a.db.Exec(ctx, `CREATE INDEX IF NOT EXISTS messages_sender_trgm ON messages USING gin (sender gin_trgm_ops)`)
	_, _ = a.db.Exec(ctx, `CREATE INDEX IF NOT EXISTS deliveries_recipient_trgm ON deliveries USING gin (recipient gin_trgm_ops)`)
	return a.seedThemes(ctx)
}

func (a *App) audit(ctx context.Context, user, action, detail string) {
	if a.db == nil {
		return
	}
	_, _ = a.db.Exec(ctx, `INSERT INTO audit_log(username, action, detail) VALUES ($1,$2,$3)`, user, action, detail)
}

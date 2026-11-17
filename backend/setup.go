package main

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) ensureSetupToken() string {
	path := filepath.Join(a.dataDir, "setup.token")
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) >= 16 {
		tok := strings.TrimSpace(string(b))
		a.announceToken(tok)
		return tok
	}
	tok := randomToken(12)
	_ = os.WriteFile(path, []byte(tok), 0o600)
	a.announceToken(tok)
	return tok
}

func (a *App) announceToken(tok string) {
	if a.setupDone() {
		return
	}
	log.Printf("========================================")
	log.Printf(" TOKEN DE CONFIGURAÇÃO INICIAL: %s", tok)
	log.Printf("========================================")
}

func (a *App) setupDone() bool {
	if !a.ready() {
		return false
	}
	var n int
	if err := a.db.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

func (a *App) checkSetup(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("X-Requested-With") != "orbita" {
		writeErr(w, 403, "requisição inválida")
		return false
	}
	if a.setupDone() {
		writeErr(w, 403, "configuração inicial já concluída")
		return false
	}
	if !a.limiter.allow("setup:" + clientIP(r)) {
		writeErr(w, 429, "muitas tentativas")
		return false
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Setup-Token")), []byte(a.setupToken)) != 1 {
		writeErr(w, 401, "token de configuração inválido (veja os logs do container)")
		return false
	}
	return true
}

func (a *App) handleSetupDB(w http.ResponseWriter, r *http.Request) {
	if !a.checkSetup(w, r) {
		return
	}
	var in dbConfig
	if !readJSON(w, r, &in) {
		return
	}
	if in.Port == 0 {
		in.Port = 5432
	}
	if in.SSLMode == "" {
		in.SSLMode = "prefer"
	}
	if in.Host == "" || in.User == "" || in.Name == "" {
		writeErr(w, 400, "informe host, usuário e nome do banco")
		return
	}
	switch in.SSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		writeErr(w, 400, "sslmode inválido")
		return
	}
	pool, err := connectDB(r.Context(), in.dsn())
	if err != nil {
		writeErr(w, 400, "não foi possível conectar: "+err.Error())
		return
	}
	if err := a.saveBootstrap(in); err != nil {
		pool.Close()
		writeErr(w, 500, "falha ao salvar configuração")
		return
	}
	a.mu.Lock()
	old := a.db
	a.mu.Unlock()
	if old != nil {
		old.Close()
	}
	if err := a.activate(context.Background(), pool); err != nil {
		writeErr(w, 500, "falha ao preparar o banco: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleSetupAdmin(w http.ResponseWriter, r *http.Request) {
	if !a.ready() {
		writeErr(w, 400, "configure o banco primeiro")
		return
	}
	if !a.checkSetup(w, r) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		AppName  string `json:"app_name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !reUser.MatchString(in.Username) {
		writeErr(w, 400, "usuário inválido")
		return
	}
	if err := validPassword(in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	h, err := hashPassword(in.Password)
	if err != nil {
		writeErr(w, 500, "falha ao processar senha")
		return
	}
	var uid int64
	err = a.db.QueryRow(r.Context(), `INSERT INTO users(username,email,role,password_hash,display_name) VALUES($1,$2,'admin',$3,'Administrador') RETURNING id`,
		in.Username, in.Email, h).Scan(&uid)
	if err != nil {
		writeErr(w, 409, "não foi possível criar o administrador")
		return
	}
	if n := strings.TrimSpace(in.AppName); n != "" && len(n) <= 60 {
		_ = a.setRaw(r.Context(), "app_name", n)
	}
	_ = a.createSession(r.Context(), w, r, uid)
	a.audit(r.Context(), in.Username, "setup.complete", "")
	_ = os.Remove(filepath.Join(a.dataDir, "setup.token"))
	writeJSON(w, 200, map[string]any{"ok": true})
}

package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	resetTTL      = 30 * time.Minute
	resetCooldown = 60 * time.Second
)

func (a *App) canReset() bool { return a.smtpReady() && a.sStr("public_url") != "" }

func (a *App) autoPublicURL(r *http.Request) {
	if a.sStr("public_url") != "" {
		return
	}
	h := r.Host
	name := h
	if hh, _, err := net.SplitHostPort(h); err == nil {
		name = hh
	}
	name = strings.Trim(name, "[]")
	if name == "" || strings.EqualFold(name, "localhost") {
		return
	}
	if ip, err := netip.ParseAddr(name); err == nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	_ = a.setRaw(r.Context(), "public_url", scheme+"://"+h)
}

type resetUser struct {
	ID       int64
	Username string
	Email    string
}

func (a *App) findResetUser(ctx context.Context, ident string) (resetUser, bool) {
	ident = strings.TrimSpace(ident)
	if ident == "" || len(ident) > 255 {
		return resetUser{}, false
	}
	rows, err := a.query(ctx, `SELECT id,username,email FROM users WHERE source='local' AND disabled=0 AND email<>'' AND (lower(username)=lower(?) OR lower(email)=lower(?)) LIMIT 2`, ident, ident)
	if err != nil {
		return resetUser{}, false
	}
	defer rows.Close()
	var us []resetUser
	for rows.Next() {
		var u resetUser
		if rows.Scan(&u.ID, &u.Username, &u.Email) == nil {
			us = append(us, u)
		}
	}
	if len(us) != 1 {
		return resetUser{}, false
	}
	return us[0], true
}

func (a *App) handleForgot(w http.ResponseWriter, r *http.Request) {
	if !a.ready() {
		writeErr(w, 503, "setup_required")
		return
	}
	if r.Header.Get("X-Requested-With") != "orbita" {
		writeErr(w, 403, "requisição inválida")
		return
	}
	if !a.canReset() {
		writeErr(w, 400, "a recuperação de senha por e-mail não está disponível neste sistema. Procure um administrador")
		return
	}
	if !a.resetRL.allow("forgot:" + clientIP(r)) {
		writeErr(w, 429, "muitas tentativas, aguarde alguns minutos")
		return
	}
	var in struct {
		Identifier string `json:"identifier"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	if u, ok := a.findResetUser(ctx, in.Identifier); ok {
		var sent time.Time
		if err := a.row(ctx, `SELECT sent_at FROM password_resets WHERE user_id=?`, u.ID).Scan(&sent); err == nil && time.Since(sent) < resetCooldown {
			writeJSON(w, 200, map[string]any{"ok": true})
			return
		}
		tok := randomToken(32)
		if _, err := a.exec(ctx, `REPLACE INTO password_resets(user_id,code_hash,attempts,sent_at,expires_at) VALUES(?,?,0,UTC_TIMESTAMP(6),?)`,
			u.ID, hashToken(tok), time.Now().UTC().Add(resetTTL)); err == nil {
			a.audit(ctx, u.Username, "password.forgot", clientIP(r))
			go func() {
				sc, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				_ = a.sendReset(sc, u, tok)
			}()
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) sendReset(ctx context.Context, u resetUser, tok string) error {
	name := a.sStr("app_name")
	link := strings.TrimRight(a.sStr("public_url"), "/") + "/redefinir-senha?t=" + url.QueryEscape(tok)
	return a.sendBranded(ctx, u.Email, "Redefinição de senha do "+name, mailMsg{
		Preheader:   "Clique para redefinir a sua senha",
		Title:       "Redefinição de senha",
		Paragraphs:  []string{fmt.Sprintf("Olá, %s. Recebemos um pedido para redefinir a senha da sua conta no %s.", u.Username, name), "Clique no botão abaixo para escolher uma nova senha. O link vale por 30 minutos e só pode ser usado uma vez."},
		ButtonLabel: "Redefinir minha senha",
		ButtonURL:   link,
		Notice:      "Se você não pediu a redefinição, ignore este e-mail: a sua senha continua a mesma. Nunca encaminhe este e-mail a ninguém.",
	})
}

func (a *App) resetByToken(ctx context.Context, tok string) (resetUser, bool) {
	tok = strings.TrimSpace(tok)
	var u resetUser
	if len(tok) < 32 || len(tok) > 128 {
		return u, false
	}
	err := a.row(ctx, `SELECT u.id,u.username,u.email FROM password_resets p JOIN users u ON u.id=p.user_id
		WHERE p.code_hash=? AND p.expires_at>UTC_TIMESTAMP(6) AND u.source='local' AND u.disabled=0`, hashToken(tok)).Scan(&u.ID, &u.Username, &u.Email)
	if err == sql.ErrNoRows || err != nil {
		return u, false
	}
	return u, true
}

const badReset = "link inválido ou expirado. Peça um novo em “Esqueci minha senha”"

func (a *App) handleResetCheck(w http.ResponseWriter, r *http.Request) {
	if !a.ready() {
		writeErr(w, 503, "setup_required")
		return
	}
	if !a.resetRL.allow("reset:" + clientIP(r)) {
		writeErr(w, 429, "muitas tentativas, aguarde alguns minutos")
		return
	}
	u, ok := a.resetByToken(r.Context(), r.URL.Query().Get("t"))
	if !ok {
		writeErr(w, 400, badReset)
		return
	}
	writeJSON(w, 200, map[string]any{"username": u.Username})
}

func (a *App) handleReset(w http.ResponseWriter, r *http.Request) {
	if !a.ready() {
		writeErr(w, 503, "setup_required")
		return
	}
	if r.Header.Get("X-Requested-With") != "orbita" {
		writeErr(w, 403, "requisição inválida")
		return
	}
	if !a.resetRL.allow("reset:" + clientIP(r)) {
		writeErr(w, 429, "muitas tentativas, aguarde alguns minutos")
		return
	}
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		Confirm  string `json:"password_confirm"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	u, ok := a.resetByToken(ctx, in.Token)
	if !ok {
		writeErr(w, 400, badReset)
		return
	}
	if err := validPassword(in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Password != in.Confirm {
		writeErr(w, 400, "a confirmação da senha não confere")
		return
	}
	nh, err := hashPassword(in.Password)
	if err != nil {
		writeErr(w, 500, "falha ao processar senha")
		return
	}
	if _, err := a.exec(ctx, `UPDATE users SET password_hash=? WHERE id=? AND source='local'`, nh, u.ID); err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	_, _ = a.exec(ctx, `DELETE FROM password_resets WHERE user_id=?`, u.ID)
	_, _ = a.exec(ctx, `DELETE FROM sessions WHERE user_id=?`, u.ID)
	_, _ = a.exec(ctx, `DELETE FROM trusted_devices WHERE user_id=?`, u.ID)
	a.audit(ctx, u.Username, "password.reset", clientIP(r))
	go func() {
		sc, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		name := a.sStr("app_name")
		_ = a.sendBranded(sc, u.Email, "Sua senha do "+name+" foi alterada", mailMsg{
			Preheader:  "A senha da sua conta foi alterada",
			Title:      "Sua senha foi alterada",
			Paragraphs: []string{fmt.Sprintf("A senha da conta %s no %s acabou de ser redefinida e todas as sessões abertas foram encerradas.", u.Username, name)},
			Notice:     "Se não foi você, procure um administrador imediatamente. Se o 2FA estiver ativo na conta, ele continua sendo exigido no login.",
		})
	}()
	writeJSON(w, 200, map[string]any{"ok": true})
}

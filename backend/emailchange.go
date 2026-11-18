package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const emailChangeTTL = time.Hour

func (a *App) publicBase(r *http.Request) string {
	if b := strings.TrimRight(a.sStr("public_url"), "/"); b != "" {
		return b
	}
	scheme := "http"
	if isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (a *App) handleEmailChangeStatus(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var email string
	var exp time.Time
	out := map[string]any{"editable": u.Source == "local", "smtp_ready": a.smtpReady(), "email": u.Email}
	if a.row(r.Context(), `SELECT new_email,expires_at FROM email_changes WHERE user_id=? AND expires_at>UTC_TIMESTAMP(6)`, u.ID).Scan(&email, &exp) == nil {
		out["pending"] = map[string]any{"email": email, "expires_at": exp}
	}
	writeJSON(w, 200, out)
}

func (a *App) handleEmailChangeRequest(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if u.Source != "local" {
		writeErr(w, 400, "o e-mail de usuários LDAP/AD vem do diretório")
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	if !a.smtpReady() {
		writeErr(w, 400, "o envio de e-mails não está configurado. Peça a um administrador (Configurações > E-mail do sistema)")
		return
	}
	if err := a.checkOwnPassword(ctx, u, in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	email := strings.TrimSpace(in.Email)
	if !reEmail.MatchString(email) || len(email) > 255 {
		writeErr(w, 400, "informe um e-mail válido")
		return
	}
	if strings.EqualFold(email, u.Email) {
		writeErr(w, 400, "este já é o seu e-mail atual")
		return
	}
	var n int
	_ = a.row(ctx, `SELECT count(*) FROM users WHERE id<>? AND (lower(email)=lower(?))`, u.ID, email).Scan(&n)
	if n > 0 {
		writeErr(w, 400, "este e-mail não pode ser usado. Escolha outro")
		return
	}
	if !a.collectRL.allow("email-change:" + fmt.Sprint(u.ID)) {
		writeErr(w, 429, "muitos pedidos. Aguarde alguns minutos")
		return
	}
	tok := randomToken(32)
	link := a.publicBase(r) + "/confirmar-email?t=" + url.QueryEscape(tok)
	name := a.sStr("app_name")
	sc, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := a.sendBranded(sc, email, "Confirme o seu novo e-mail no "+name, mailMsg{
		Preheader:   "Confirme a alteração do e-mail da sua conta",
		Title:       "Confirme o seu novo e-mail",
		Paragraphs:  []string{fmt.Sprintf("Olá, %s. Você pediu para usar este endereço como e-mail da sua conta no %s.", u.Username, name), "Clique no botão abaixo para confirmar a alteração. O link vale por 1 hora."},
		ButtonLabel: "Confirmar novo e-mail",
		ButtonURL:   link,
		Notice:      "Se não foi você, ignore este e-mail: nada será alterado e o e-mail atual da conta continua o mesmo.",
	}); err != nil {
		writeErr(w, 502, "não foi possível enviar o e-mail de confirmação: "+err.Error())
		return
	}
	if _, err := a.exec(ctx, `REPLACE INTO email_changes(user_id,new_email,token_hash,expires_at) VALUES(?,?,?,?)`,
		u.ID, email, hashToken(tok), time.Now().UTC().Add(emailChangeTTL)); err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	a.audit(ctx, u.Username, "email.change.request", maskEmail(email))
	writeJSON(w, 200, map[string]any{"ok": true, "email_hint": maskEmail(email)})
}

func (a *App) handleEmailChangeCancel(w http.ResponseWriter, r *http.Request) {
	_, _ = a.exec(r.Context(), `DELETE FROM email_changes WHERE user_id=?`, userFrom(r).ID)
	writeJSON(w, 200, map[string]any{"ok": true})
}

type pendingChange struct {
	uid      int64
	username string
	old, neu string
}

func (a *App) findChange(ctx context.Context, tok string) (pendingChange, bool) {
	var p pendingChange
	var h string
	tok = strings.TrimSpace(tok)
	if len(tok) < 32 || len(tok) > 128 {
		return p, false
	}
	err := a.row(ctx, `SELECT c.user_id,u.username,u.email,c.new_email,c.token_hash FROM email_changes c JOIN users u ON u.id=c.user_id
		WHERE c.token_hash=? AND c.expires_at>UTC_TIMESTAMP(6) AND u.source='local' AND u.disabled=0`, hashToken(tok)).Scan(&p.uid, &p.username, &p.old, &p.neu, &h)
	if err != nil || subtle.ConstantTimeCompare([]byte(h), []byte(hashToken(tok))) != 1 {
		return p, false
	}
	return p, true
}

func (a *App) handleEmailConfirmPreview(w http.ResponseWriter, r *http.Request) {
	if !a.ready() {
		writeErr(w, 503, "setup_required")
		return
	}
	if !a.resetRL.allow("econfirm:" + clientIP(r)) {
		writeErr(w, 429, "muitas tentativas, aguarde alguns minutos")
		return
	}
	p, ok := a.findChange(r.Context(), r.URL.Query().Get("t"))
	if !ok {
		writeErr(w, 400, "link inválido ou expirado. Peça a alteração novamente em Minha conta")
		return
	}
	writeJSON(w, 200, map[string]any{"username": p.username, "email": p.neu})
}

func (a *App) handleEmailConfirm(w http.ResponseWriter, r *http.Request) {
	if !a.ready() {
		writeErr(w, 503, "setup_required")
		return
	}
	if r.Header.Get("X-Requested-With") != "orbita" {
		writeErr(w, 403, "requisição inválida")
		return
	}
	if !a.resetRL.allow("econfirm:" + clientIP(r)) {
		writeErr(w, 429, "muitas tentativas, aguarde alguns minutos")
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	p, ok := a.findChange(ctx, in.Token)
	if !ok {
		writeErr(w, 400, "link inválido ou expirado. Peça a alteração novamente em Minha conta")
		return
	}
	var n int
	_ = a.row(ctx, `SELECT count(*) FROM users WHERE id<>? AND lower(email)=lower(?)`, p.uid, p.neu).Scan(&n)
	if n > 0 {
		_, _ = a.exec(ctx, `DELETE FROM email_changes WHERE user_id=?`, p.uid)
		writeErr(w, 400, "este e-mail não pode mais ser usado. Peça a alteração com outro endereço")
		return
	}
	if _, err := a.exec(ctx, `UPDATE users SET email=? WHERE id=?`, p.neu, p.uid); err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	_, _ = a.exec(ctx, `DELETE FROM email_changes WHERE user_id=?`, p.uid)
	a.audit(ctx, p.username, "email.change", maskEmail(p.neu))
	if p.old != "" && a.smtpReady() {
		go func() {
			sc, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			name := a.sStr("app_name")
			_ = a.sendBranded(sc, p.old, "O e-mail da sua conta no "+name+" foi alterado", mailMsg{
				Preheader:  "O e-mail da sua conta foi alterado",
				Title:      "O e-mail da sua conta foi alterado",
				Paragraphs: []string{fmt.Sprintf("O e-mail da conta %s no %s foi alterado para %s.", p.username, name, maskEmail(p.neu))},
				Notice:     "Se não foi você, procure um administrador imediatamente e troque a sua senha.",
			})
		}()
	}
	writeJSON(w, 200, map[string]any{"ok": true, "email": p.neu})
}

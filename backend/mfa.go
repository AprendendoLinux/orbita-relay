package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"crypto/tls"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	mfaChallengeTTL = 10 * time.Minute
	mfaMaxAttempts  = 5
	mfaCodeCooldown = 30 * time.Second
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func newTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return b32.EncodeToString(b)
}

func hotp(secret []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	h := hmac.New(sha1.New, secret)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off]&0x7f) << 24) | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", code%1000000)
}

func totpMatch(secret, code string, now time.Time, last int64) (int64, bool) {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return 0, false
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0, false
	}
	step := now.Unix() / 30
	for d := int64(-1); d <= 1; d++ {
		s := step + d
		if s <= last {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(hotp(key, uint64(s))), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

func numericCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	return fmt.Sprintf("%06d", n.Int64())
}

const recAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func newRecoveryCode() string {
	b := make([]byte, 10)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(recAlphabet))))
		b[i] = recAlphabet[n.Int64()]
	}
	return string(b[:5]) + "-" + string(b[5:])
}

func normRecovery(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
}

func (a *App) regenRecovery(ctx context.Context, uid int64) ([]string, error) {
	if _, err := a.exec(ctx, `DELETE FROM mfa_recovery WHERE user_id=?`, uid); err != nil {
		return nil, err
	}
	out := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		c := newRecoveryCode()
		if _, err := a.exec(ctx, `INSERT INTO mfa_recovery(user_id,code_hash) VALUES(?,?)`, uid, hashToken(normRecovery(c))); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (a *App) useRecovery(ctx context.Context, uid int64, code string) bool {
	n := normRecovery(code)
	if len(n) != 10 {
		return false
	}
	res, err := a.exec(ctx, `DELETE FROM mfa_recovery WHERE user_id=? AND code_hash=?`, uid, hashToken(n))
	if err != nil {
		return false
	}
	c, _ := res.RowsAffected()
	return c > 0
}

type mfaUser struct {
	ID       int64
	Username string
	Source   string
	Method   string
	Secret   string
	Email    string
	Last     int64
}

func (a *App) loadMFA(ctx context.Context, uid int64) (mfaUser, error) {
	var u mfaUser
	var sec, em sql.NullString
	err := a.row(ctx, `SELECT id,username,source,mfa_method,totp_secret,COALESCE(NULLIF(email,''),mfa_email),totp_last FROM users WHERE id=?`, uid).
		Scan(&u.ID, &u.Username, &u.Source, &u.Method, &sec, &em, &u.Last)
	u.Secret, u.Email = sec.String, em.String
	return u, err
}

func maskEmail(e string) string {
	i := strings.IndexByte(e, '@')
	if i < 1 {
		return ""
	}
	n := e[:i]
	if len(n) <= 2 {
		return n[:1] + "***" + e[i:]
	}
	return n[:2] + strings.Repeat("*", len(n)-2) + e[i:]
}

func (a *App) mfaVerify(ctx context.Context, u mfaUser, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	if u.Method == "totp" && u.Secret != "" {
		if sec, err := a.decrypt(u.Secret); err == nil {
			if step, ok := totpMatch(sec, code, time.Now(), u.Last); ok {
				_, _ = a.exec(ctx, `UPDATE users SET totp_last=? WHERE id=?`, step, u.ID)
				return true
			}
		}
	}
	return a.useRecovery(ctx, u.ID, code)
}

func (a *App) smtpReady() bool { return a.sStr("smtp_host") != "" && a.sStr("smtp_from") != "" }

type smtpCfg struct {
	Host, TLS, User, Pass, From string
	Port                        int
	Skip                        bool
}

func (a *App) smtpSaved() smtpCfg {
	return smtpCfg{Host: a.sStr("smtp_host"), Port: a.sInt("smtp_port"), TLS: a.sStr("smtp_tls"), User: a.sStr("smtp_user"),
		Pass: a.sStr("smtp_pass"), From: a.sStr("smtp_from"), Skip: a.sBool("smtp_skip_verify")}
}

func (a *App) smtpDeliver(ctx context.Context, cfg smtpCfg, to string, raw []byte) error {
	host, port, mode := cfg.Host, cfg.Port, cfg.TLS
	from := cfg.From
	if host == "" || from == "" {
		return errors.New("o servidor SMTP de saída não está configurado (preencha servidor e remetente em Configurações > E-mail do sistema)")
	}
	if strings.ContainsAny(to+from, "\r\n") {
		return errors.New("endereço inválido")
	}
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.Skip}
	d := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if mode == "tls" {
		conn, err = tls.DialWithDialer(d, "tcp", addr, tlsCfg)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("não foi possível conectar ao SMTP: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if mode == "starttls" {
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS falhou: %w", err)
		}
	}
	if u := cfg.User; u != "" {
		if mode == "none" {
			return errors.New("para usar usuário e senha, ative STARTTLS ou TLS")
		}
		if err := c.Auth(smtp.PlainAuth("", u, cfg.Pass, host)); err != nil {
			return fmt.Errorf("autenticação SMTP recusada: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func (a *App) sendCode(ctx context.Context, to, code, purpose string) error {
	name := a.sStr("app_name")
	return a.sendBranded(ctx, to, fmt.Sprintf("%s é o seu código de verificação do %s", code, name), mailMsg{
		Preheader:  "Seu código de verificação: " + code,
		Title:      "Seu código de verificação",
		Paragraphs: []string{fmt.Sprintf("Use o código abaixo para concluir o seu %s no %s.", purpose, name)},
		Code:       code,
		CodeNote:   "O código vale por 10 minutos e só pode ser usado uma vez.",
		Notice:     "Se não foi você quem pediu este código, ignore esta mensagem e considere trocar a sua senha. Nunca compartilhe este código com ninguém.",
	})
}

func (a *App) handleTestSMTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		To   string  `json:"to"`
		Host *string `json:"host"`
		Port *int    `json:"port"`
		TLS  *string `json:"tls"`
		User *string `json:"user"`
		Pass *string `json:"pass"`
		From *string `json:"from"`
		Skip *bool   `json:"skip_verify"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	to := strings.TrimSpace(in.To)
	if !reEmail.MatchString(to) {
		writeErr(w, 400, "informe um e-mail de destino válido")
		return
	}
	cfg := a.smtpSaved()
	if in.Host != nil {
		cfg.Host = strings.TrimSpace(*in.Host)
	}
	if in.Port != nil && *in.Port > 0 && *in.Port < 65536 {
		cfg.Port = *in.Port
	}
	if in.TLS != nil && (*in.TLS == "none" || *in.TLS == "starttls" || *in.TLS == "tls") {
		cfg.TLS = *in.TLS
	}
	if in.User != nil {
		cfg.User = strings.TrimSpace(*in.User)
	}
	if in.Pass != nil && *in.Pass != "" {
		cfg.Pass = *in.Pass
	}
	if in.From != nil {
		cfg.From = strings.TrimSpace(*in.From)
	}
	if in.Skip != nil {
		cfg.Skip = *in.Skip
	}
	if strings.ContainsAny(cfg.Host+cfg.User+cfg.Pass, "\r\n") || (cfg.From != "" && !reEmail.MatchString(cfg.From)) {
		writeErr(w, 400, "dados do SMTP inválidos")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := a.sendBrandedCfg(ctx, cfg, to, "Teste de e-mail do "+a.sStr("app_name"), mailMsg{
		Preheader:  "O envio de e-mails do sistema está funcionando",
		Title:      "Tudo certo com o envio de e-mails",
		Paragraphs: []string{"Se você está lendo esta mensagem, o servidor SMTP configurado no " + a.sStr("app_name") + " está funcionando corretamente.", "Os códigos de verificação em duas etapas e a recuperação de senha serão enviados com esta mesma aparência."},
		Notice:     "Este é apenas um e-mail de teste. Nenhuma ação é necessária.",
	}); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

const (
	trustCookie = "orbita_td"
	trustTTL    = 30 * 24 * time.Hour
)

func (a *App) isTrustedDevice(ctx context.Context, r *http.Request, uid int64) bool {
	c, err := r.Cookie(trustCookie)
	if err != nil || len(c.Value) < 32 {
		return false
	}
	var n int
	_ = a.row(ctx, `SELECT count(*) FROM trusted_devices WHERE token_hash=? AND user_id=? AND expires_at>UTC_TIMESTAMP(6)`, hashToken(c.Value), uid).Scan(&n)
	return n > 0
}

func (a *App) trustDevice(ctx context.Context, w http.ResponseWriter, r *http.Request, uid int64) {
	tok := randomToken(32)
	exp := time.Now().Add(trustTTL)
	if _, err := a.exec(ctx, `INSERT INTO trusted_devices(token_hash,user_id,expires_at,ip) VALUES(?,?,?,?)`, hashToken(tok), uid, exp.UTC(), clientIP(r)); err != nil {
		return
	}
	_, _ = a.exec(ctx, `DELETE FROM trusted_devices WHERE expires_at<UTC_TIMESTAMP(6)`)
	http.SetCookie(w, &http.Cookie{Name: trustCookie, Value: tok, Path: "/", Expires: exp, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode})
}

func (a *App) startMFA(ctx context.Context, w http.ResponseWriter, r *http.Request, uid int64) bool {
	u, err := a.loadMFA(ctx, uid)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return true
	}
	if u.Method == "" {
		return false
	}
	if a.isTrustedDevice(ctx, r, uid) {
		return false
	}
	resp := map[string]any{"mfa_required": true, "method": u.Method}
	hash := ""
	var code string
	if u.Method == "email" {
		code = numericCode()
	}
	tok := randomToken(32)
	if code != "" {
		hash = hashToken(code + tok)
	}
	_, _ = a.exec(ctx, `DELETE FROM mfa_challenges WHERE user_id=? OR expires_at<UTC_TIMESTAMP(6)`, uid)
	if _, err := a.exec(ctx, `INSERT INTO mfa_challenges(token_hash,user_id,method,code_hash,expires_at,sent_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(6))`,
		hashToken(tok), uid, u.Method, hash, time.Now().UTC().Add(mfaChallengeTTL)); err != nil {
		writeErr(w, 500, "erro de banco")
		return true
	}
	if code != "" {
		sc, cancel := context.WithTimeout(ctx, 40*time.Second)
		defer cancel()
		if err := a.sendCode(sc, u.Email, code, "acesso"); err != nil {
			_, _ = a.exec(ctx, `DELETE FROM mfa_challenges WHERE token_hash=?`, hashToken(tok))
			writeErr(w, 502, "não foi possível enviar o código por e-mail: "+err.Error()+". Peça a um administrador para redefinir o seu 2FA")
			return true
		}
		resp["email_hint"] = maskEmail(u.Email)
	}
	resp["mfa_token"] = tok
	writeJSON(w, 200, resp)
	return true
}

func (a *App) handleLoginMFA(w http.ResponseWriter, r *http.Request) {
	if !a.ready() {
		writeErr(w, 503, "setup_required")
		return
	}
	if r.Header.Get("X-Requested-With") != "orbita" {
		writeErr(w, 403, "requisição inválida")
		return
	}
	if !a.limiter.allow(clientIP(r)) {
		writeErr(w, 429, "muitas tentativas, aguarde alguns minutos")
		return
	}
	var in struct {
		Token    string `json:"mfa_token"`
		Code     string `json:"code"`
		Remember bool   `json:"remember"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	th := hashToken(in.Token)
	var uid int64
	var method, codeHash string
	var attempts int
	err := a.row(ctx, `SELECT user_id,method,COALESCE(code_hash,''),attempts FROM mfa_challenges WHERE token_hash=? AND expires_at>UTC_TIMESTAMP(6)`, th).
		Scan(&uid, &method, &codeHash, &attempts)
	if err != nil {
		writeErr(w, 401, "a verificação expirou. Entre novamente")
		return
	}
	if attempts >= mfaMaxAttempts {
		_, _ = a.exec(ctx, `DELETE FROM mfa_challenges WHERE token_hash=?`, th)
		writeErr(w, 401, "muitas tentativas. Entre novamente")
		return
	}
	u, err := a.loadMFA(ctx, uid)
	if err != nil {
		writeErr(w, 401, "a verificação expirou. Entre novamente")
		return
	}
	code := strings.TrimSpace(in.Code)
	ok := false
	if method == "email" && codeHash != "" && len(code) == 6 {
		ok = subtle.ConstantTimeCompare([]byte(hashToken(code+in.Token)), []byte(codeHash)) == 1
	}
	if !ok {
		ok = a.mfaVerify(ctx, u, code)
	}
	if !ok {
		_, _ = a.exec(ctx, `UPDATE mfa_challenges SET attempts=attempts+1 WHERE token_hash=?`, th)
		a.audit(ctx, u.Username, "login.mfa.fail", clientIP(r))
		writeErr(w, 401, "código inválido")
		return
	}
	_, _ = a.exec(ctx, `DELETE FROM mfa_challenges WHERE user_id=?`, uid)
	if err := a.createSession(ctx, w, r, uid); err != nil {
		writeErr(w, 500, "falha ao criar sessão")
		return
	}
	if in.Remember {
		a.trustDevice(ctx, w, r, uid)
	}
	a.audit(ctx, u.Username, "login.ok", clientIP(r)+" (2FA)")
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleLoginMFAResend(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Requested-With") != "orbita" {
		writeErr(w, 403, "requisição inválida")
		return
	}
	if !a.limiter.allow(clientIP(r)) {
		writeErr(w, 429, "muitas tentativas, aguarde alguns minutos")
		return
	}
	var in struct {
		Token string `json:"mfa_token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	th := hashToken(in.Token)
	var uid int64
	var method string
	var sent time.Time
	if a.row(ctx, `SELECT user_id,method,sent_at FROM mfa_challenges WHERE token_hash=? AND expires_at>UTC_TIMESTAMP(6)`, th).Scan(&uid, &method, &sent) != nil || method != "email" {
		writeErr(w, 401, "a verificação expirou. Entre novamente")
		return
	}
	if time.Since(sent) < mfaCodeCooldown {
		writeErr(w, 429, "aguarde alguns segundos para reenviar")
		return
	}
	u, err := a.loadMFA(ctx, uid)
	if err != nil {
		writeErr(w, 401, "a verificação expirou. Entre novamente")
		return
	}
	code := numericCode()
	sc, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := a.sendCode(sc, u.Email, code, "acesso"); err != nil {
		writeErr(w, 502, "não foi possível enviar o código: "+err.Error())
		return
	}
	_, _ = a.exec(ctx, `UPDATE mfa_challenges SET code_hash=?, attempts=0, sent_at=UTC_TIMESTAMP(6) WHERE token_hash=?`, hashToken(code+in.Token), th)
	writeJSON(w, 200, map[string]any{"ok": true, "email_hint": maskEmail(u.Email)})
}

func (a *App) checkOwnPassword(ctx context.Context, u *User, pw string) error {
	if u.Source != "local" {
		return nil
	}
	var hash string
	if err := a.row(ctx, `SELECT COALESCE(password_hash,'') FROM users WHERE id=?`, u.ID).Scan(&hash); err != nil ||
		hash == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) != nil {
		return errors.New("senha atual incorreta")
	}
	return nil
}

func (a *App) handleMFAStatus(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	m, err := a.loadMFA(r.Context(), u.ID)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	var left, trusted int
	_ = a.row(r.Context(), `SELECT count(*) FROM mfa_recovery WHERE user_id=?`, u.ID).Scan(&left)
	_ = a.row(r.Context(), `SELECT count(*) FROM trusted_devices WHERE user_id=? AND expires_at>UTC_TIMESTAMP(6)`, u.ID).Scan(&trusted)
	writeJSON(w, 200, map[string]any{
		"method": m.Method, "email": maskEmail(m.Email), "recovery_left": left, "smtp_ready": a.smtpReady(),
		"needs_password": u.Source == "local", "user_email": u.Email, "trusted_devices": trusted,
	})
}

func (a *App) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := a.checkOwnPassword(r.Context(), u, in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	secret := newTOTPSecret()
	enc, err := a.encrypt(secret)
	if err != nil {
		writeErr(w, 500, "erro ao proteger o segredo")
		return
	}
	if _, err := a.exec(r.Context(), `REPLACE INTO mfa_pending(user_id,kind,secret,email,expires_at) VALUES(?,?,?,?,?)`,
		u.ID, "totp", enc, "", time.Now().UTC().Add(30*time.Minute)); err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	issuer := a.sStr("app_name")
	uri := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30",
		url.PathEscape(issuer), url.PathEscape(u.Username), secret, url.QueryEscape(issuer))
	writeJSON(w, 200, map[string]any{"secret": secret, "uri": uri})
}

func (a *App) pending(ctx context.Context, uid int64, kind string) (secret, email string, attempts int, err error) {
	err = a.row(ctx, `SELECT secret,email,attempts FROM mfa_pending WHERE user_id=? AND kind=? AND expires_at>UTC_TIMESTAMP(6)`, uid, kind).Scan(&secret, &email, &attempts)
	return
}

func (a *App) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	enc, _, attempts, err := a.pending(ctx, u.ID, "totp")
	if err != nil {
		writeErr(w, 400, "inicie a configuração novamente")
		return
	}
	if attempts >= 8 {
		_, _ = a.exec(ctx, `DELETE FROM mfa_pending WHERE user_id=?`, u.ID)
		writeErr(w, 400, "muitas tentativas. Inicie a configuração novamente")
		return
	}
	sec, err := a.decrypt(enc)
	if err != nil {
		writeErr(w, 500, "erro ao ler o segredo")
		return
	}
	step, ok := totpMatch(sec, in.Code, time.Now(), 0)
	if !ok {
		_, _ = a.exec(ctx, `UPDATE mfa_pending SET attempts=attempts+1 WHERE user_id=? AND kind='totp'`, u.ID)
		writeErr(w, 400, "código inválido. Confira o horário do celular e tente de novo")
		return
	}
	if _, err := a.exec(ctx, `UPDATE users SET mfa_method='totp', totp_secret=?, totp_last=? WHERE id=?`, enc, step, u.ID); err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	_, _ = a.exec(ctx, `DELETE FROM mfa_pending WHERE user_id=?`, u.ID)
	codes, err := a.regenRecovery(ctx, u.ID)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	a.audit(ctx, u.Username, "mfa.enable", "totp")
	writeJSON(w, 200, map[string]any{"ok": true, "recovery": codes})
}

func (a *App) handleEmailSetup(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
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
	email := strings.TrimSpace(u.Email)
	if !reEmail.MatchString(email) {
		writeErr(w, 400, "a sua conta não tem um e-mail cadastrado. Cadastre um em Minha conta (ou peça a um administrador) antes de usar o código por e-mail")
		return
	}
	if !a.collectRL.allow("mfa-mail:" + fmt.Sprint(u.ID)) {
		writeErr(w, 429, "muitos envios. Aguarde alguns minutos")
		return
	}
	code := numericCode()
	sc, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := a.sendCode(sc, email, code, "confirmação do 2FA"); err != nil {
		writeErr(w, 502, "não foi possível enviar o código: "+err.Error())
		return
	}
	if _, err := a.exec(ctx, `REPLACE INTO mfa_pending(user_id,kind,secret,email,expires_at) VALUES(?,?,?,?,?)`,
		u.ID, "email", hashToken(code+fmt.Sprint(u.ID)), email, time.Now().UTC().Add(mfaChallengeTTL)); err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "email_hint": maskEmail(email)})
}

func (a *App) handleEmailEnable(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	h, email, attempts, err := a.pending(ctx, u.ID, "email")
	if err != nil {
		writeErr(w, 400, "peça o código novamente")
		return
	}
	if attempts >= mfaMaxAttempts {
		_, _ = a.exec(ctx, `DELETE FROM mfa_pending WHERE user_id=?`, u.ID)
		writeErr(w, 400, "muitas tentativas. Peça um novo código")
		return
	}
	if subtle.ConstantTimeCompare([]byte(hashToken(strings.TrimSpace(in.Code)+fmt.Sprint(u.ID))), []byte(h)) != 1 {
		_, _ = a.exec(ctx, `UPDATE mfa_pending SET attempts=attempts+1 WHERE user_id=? AND kind='email'`, u.ID)
		writeErr(w, 400, "código inválido")
		return
	}
	if _, err := a.exec(ctx, `UPDATE users SET mfa_method='email', mfa_email=?, totp_secret=NULL, totp_last=0 WHERE id=?`, email, u.ID); err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	_, _ = a.exec(ctx, `DELETE FROM mfa_pending WHERE user_id=?`, u.ID)
	codes, err := a.regenRecovery(ctx, u.ID)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	a.audit(ctx, u.Username, "mfa.enable", "email")
	writeJSON(w, 200, map[string]any{"ok": true, "recovery": codes})
}

func (a *App) reauth(w http.ResponseWriter, r *http.Request) (*User, mfaUser, string, bool) {
	u := userFrom(r)
	var in struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return nil, mfaUser{}, "", false
	}
	ctx := r.Context()
	if err := a.checkOwnPassword(ctx, u, in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return nil, mfaUser{}, "", false
	}
	m, err := a.loadMFA(ctx, u.ID)
	if err != nil || m.Method == "" {
		writeErr(w, 400, "o 2FA não está ativo")
		return nil, mfaUser{}, "", false
	}
	if !a.collectRL.allow("mfa-reauth:" + fmt.Sprint(u.ID)) {
		writeErr(w, 429, "muitas tentativas. Aguarde alguns minutos")
		return nil, mfaUser{}, "", false
	}
	if !a.mfaVerify(ctx, m, in.Code) {
		writeErr(w, 400, "código inválido")
		return nil, mfaUser{}, "", false
	}
	return u, m, in.Code, true
}

func (a *App) handleMFADisable(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	if err := a.checkOwnPassword(ctx, u, in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !a.collectRL.allow("mfa-reauth:" + fmt.Sprint(u.ID)) {
		writeErr(w, 429, "muitas tentativas. Aguarde alguns minutos")
		return
	}
	a.clearMFA(ctx, u.ID)
	a.audit(ctx, u.Username, "mfa.disable", "")
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleTrustedRevoke(w http.ResponseWriter, r *http.Request) {
	_, _ = a.exec(r.Context(), `DELETE FROM trusted_devices WHERE user_id=?`, userFrom(r).ID)
	a.audit(r.Context(), userFrom(r).Username, "mfa.trusted.revoke", "")
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleRecoveryRegen(w http.ResponseWriter, r *http.Request) {
	u, _, _, ok := a.reauth(w, r)
	if !ok {
		return
	}
	codes, err := a.regenRecovery(r.Context(), u.ID)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	a.audit(r.Context(), u.Username, "mfa.recovery", "")
	writeJSON(w, 200, map[string]any{"ok": true, "recovery": codes})
}

func (a *App) clearMFA(ctx context.Context, uid int64) {
	_, _ = a.exec(ctx, `UPDATE users SET mfa_method='', totp_secret=NULL, mfa_email='', totp_last=0 WHERE id=?`, uid)
	_, _ = a.exec(ctx, `DELETE FROM mfa_recovery WHERE user_id=?`, uid)
	_, _ = a.exec(ctx, `DELETE FROM mfa_pending WHERE user_id=?`, uid)
	_, _ = a.exec(ctx, `DELETE FROM mfa_challenges WHERE user_id=?`, uid)
	_, _ = a.exec(ctx, `DELETE FROM trusted_devices WHERE user_id=?`, uid)
}

func (a *App) handleAdminMFAReset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var uid int64
	var name string
	if err := a.row(r.Context(), `SELECT id,username FROM users WHERE id=?`, id).Scan(&uid, &name); err != nil {
		writeErr(w, 404, "usuário não encontrado")
		return
	}
	a.clearMFA(r.Context(), uid)
	_, _ = a.exec(r.Context(), `DELETE FROM sessions WHERE user_id=? AND user_id<>?`, uid, userFrom(r).ID)
	a.audit(r.Context(), userFrom(r).Username, "mfa.reset", name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

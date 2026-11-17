package main

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const cookieName = "orbita_session"
const sessionTTL = 12 * time.Hour

type User struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Source      string     `json:"source"`
	Disabled    bool       `json:"disabled"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLogin   *time.Time `json:"last_login"`
}

type ctxKey int

const userKey ctxKey = 1

var roleRank = map[string]int{"viewer": 1, "operator": 2, "admin": 3}

func userFrom(r *http.Request) *User {
	u, _ := r.Context().Value(userKey).(*User)
	if u == nil {
		return &User{}
	}
	return u
}

type rateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *rateLimiter) over(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, t := range l.hits[key] {
		if time.Since(t) < l.window {
			n++
		}
	}
	return n >= l.max
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

var trustProxy atomic.Bool

func (a *App) applyRuntime() { trustProxy.Store(a.sBool("trust_proxy")) }

func clientIP(r *http.Request) string {
	if trustProxy.Load() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip, err := netip.ParseAddr(strings.TrimSpace(parts[len(parts)-1])); err == nil {
				return ip.String()
			}
		}
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *App) createSession(ctx context.Context, w http.ResponseWriter, r *http.Request, uid int64) error {
	tok := randomToken(32)
	exp := time.Now().Add(sessionTTL)
	_, err := a.exec(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at,ip) VALUES(?,?,?,?)`,
		hashToken(tok), uid, exp, clientIP(r))
	if err != nil {
		return err
	}
	_, _ = a.exec(ctx, `UPDATE users SET last_login=UTC_TIMESTAMP(6) WHERE id=?`, uid)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: "/", Expires: exp,
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (a *App) userFromRequest(r *http.Request) *User {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	var u User
	err = a.row(r.Context(), `
		SELECT u.id,u.username,u.display_name,u.email,u.role,u.source,u.disabled,u.created_at,u.last_login
		FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=? AND s.expires_at>UTC_TIMESTAMP(6) AND u.disabled=0`, hashToken(c.Value)).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Source, &u.Disabled, &u.CreatedAt, &u.LastLogin)
	if err != nil {
		return nil
	}
	return &u
}

func (a *App) auth(min string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.ready() {
			writeErr(w, 503, "setup_required")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Requested-With") != "orbita" {
			writeErr(w, 403, "requisição inválida")
			return
		}
		u := a.userFromRequest(r)
		if u == nil {
			writeErr(w, 401, "não autenticado")
			return
		}
		if roleRank[u.Role] < roleRank[min] {
			writeErr(w, 403, "sem permissão")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	}
}

func hashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), 12)
	return string(b), err
}

func validPassword(p string) error {
	if len(p) < 10 {
		return errors.New("a senha deve ter ao menos 10 caracteres")
	}
	if len(p) > 72 {
		return errors.New("a senha deve ter no máximo 72 caracteres")
	}
	return nil
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
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
	var in struct{ Username, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || in.Password == "" || len(in.Username) > 128 {
		writeErr(w, 400, "informe usuário e senha")
		return
	}
	ctx := r.Context()
	uid, ok := a.authenticate(ctx, in.Username, in.Password)
	if !ok {
		a.audit(ctx, in.Username, "login.fail", clientIP(r))
		writeErr(w, 401, "usuário ou senha inválidos")
		return
	}
	if err := a.createSession(ctx, w, r, uid); err != nil {
		writeErr(w, 500, "falha ao criar sessão")
		return
	}
	a.audit(ctx, in.Username, "login.ok", clientIP(r))
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) authenticate(ctx context.Context, username, password string) (int64, bool) {
	var id int64
	var hash *string
	var source string
	var disabled bool
	err := a.row(ctx, `SELECT id,password_hash,source,disabled FROM users WHERE lower(username)=lower(?)`, username).
		Scan(&id, &hash, &source, &disabled)
	if err == nil && source == "local" && hash != nil {
		if disabled {
			return 0, false
		}
		if bcrypt.CompareHashAndPassword([]byte(*hash), []byte(password)) == nil {
			return id, true
		}
		return 0, false
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false
	}
	if a.sStr("auth_mode") != "ldap" {
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$0000000000000000000000000000000000000000000000000000"), []byte(password))
		return 0, false
	}
	info, ok := a.ldapAuth(username, password)
	if !ok {
		return 0, false
	}
	if disabled {
		return 0, false
	}
	role := "viewer"
	if info.admin {
		role = "admin"
	}
	if errors.Is(err, sql.ErrNoRows) {
		res, ierr := a.exec(ctx, `INSERT INTO users(username,display_name,email,role,source) VALUES(?,?,?,?,'ldap')
			ON DUPLICATE KEY UPDATE display_name=VALUES(display_name), id=LAST_INSERT_ID(id)`,
			username, info.name, info.mail, role)
		if ierr != nil {
			return 0, false
		}
		id, _ = res.LastInsertId()
		return id, id > 0
	}
	_, _ = a.exec(ctx, `UPDATE users SET display_name=?,email=? WHERE id=?`, info.name, info.mail, id)
	if info.admin {
		_, _ = a.exec(ctx, `UPDATE users SET role='admin' WHERE id=?`, id)
	}
	return id, true
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil && a.ready() {
		_, _ = a.exec(r.Context(), `DELETE FROM sessions WHERE token_hash=?`, hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, userFrom(r))
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if u.Source != "local" {
		writeErr(w, 400, "usuários LDAP alteram a senha no diretório")
		return
	}
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := validPassword(in.New); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var hash string
	if err := a.row(r.Context(), `SELECT password_hash FROM users WHERE id=?`, u.ID).Scan(&hash); err != nil ||
		bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Current)) != nil {
		writeErr(w, 400, "senha atual incorreta")
		return
	}
	nh, err := hashPassword(in.New)
	if err != nil {
		writeErr(w, 500, "falha ao processar senha")
		return
	}
	_, _ = a.exec(r.Context(), `UPDATE users SET password_hash=? WHERE id=?`, nh, u.ID)
	c, _ := r.Cookie(cookieName)
	_, _ = a.exec(r.Context(), `DELETE FROM sessions WHERE user_id=? AND token_hash<>?`, u.ID, hashToken(c.Value))
	a.audit(r.Context(), u.Username, "user.password", "")
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.query(r.Context(), `SELECT id,username,display_name,email,role,source,disabled,created_at,last_login FROM users ORDER BY username`)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Role, &u.Source, &u.Disabled, &u.CreatedAt, &u.LastLogin) == nil {
			out = append(out, u)
		}
	}
	writeJSON(w, 200, out)
}

var reUser = regexp.MustCompile(`^[A-Za-z0-9._@-]{2,64}$`)

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username, DisplayName, Email, Role, Password string
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !reUser.MatchString(in.Username) {
		writeErr(w, 400, "usuário inválido (2-64 caracteres: letras, números, . _ @ -)")
		return
	}
	if roleRank[in.Role] == 0 {
		writeErr(w, 400, "perfil inválido")
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
	_, err = a.exec(r.Context(), `INSERT INTO users(username,display_name,email,role,password_hash) VALUES(?,?,?,?,?)`,
		in.Username, in.DisplayName, in.Email, in.Role, h)
	if err != nil {
		writeErr(w, 409, "usuário já existe")
		return
	}
	a.audit(r.Context(), userFrom(r).Username, "user.create", in.Username)
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
		Role        *string `json:"role"`
		Disabled    *bool   `json:"disabled"`
		Password    *string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	me := userFrom(r)
	var target User
	if err := a.row(r.Context(), `SELECT id,username,source FROM users WHERE id=?`, id).Scan(&target.ID, &target.Username, &target.Source); err != nil {
		writeErr(w, 404, "usuário não encontrado")
		return
	}
	if target.ID == me.ID && ((in.Disabled != nil && *in.Disabled) || (in.Role != nil && *in.Role != "admin")) {
		writeErr(w, 400, "você não pode remover o próprio acesso de administrador")
		return
	}
	ctx := r.Context()
	if in.DisplayName != nil {
		_, _ = a.exec(ctx, `UPDATE users SET display_name=? WHERE id=?`, *in.DisplayName, id)
	}
	if in.Email != nil {
		_, _ = a.exec(ctx, `UPDATE users SET email=? WHERE id=?`, *in.Email, id)
	}
	if in.Role != nil {
		if roleRank[*in.Role] == 0 {
			writeErr(w, 400, "perfil inválido")
			return
		}
		_, _ = a.exec(ctx, `UPDATE users SET role=? WHERE id=?`, *in.Role, id)
	}
	if in.Disabled != nil {
		_, _ = a.exec(ctx, `UPDATE users SET disabled=? WHERE id=?`, *in.Disabled, id)
		if *in.Disabled {
			_, _ = a.exec(ctx, `DELETE FROM sessions WHERE user_id=?`, id)
		}
	}
	if in.Password != nil && *in.Password != "" {
		if target.Source != "local" {
			writeErr(w, 400, "usuário LDAP não possui senha local")
			return
		}
		if err := validPassword(*in.Password); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		h, _ := hashPassword(*in.Password)
		_, _ = a.exec(ctx, `UPDATE users SET password_hash=? WHERE id=?`, h, id)
		_, _ = a.exec(ctx, `DELETE FROM sessions WHERE user_id=?`, id)
	}
	a.audit(ctx, me.Username, "user.update", target.Username)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	me := userFrom(r)
	var name string
	if err := a.row(r.Context(), `SELECT username FROM users WHERE id=?`, id).Scan(&name); err != nil {
		writeErr(w, 404, "usuário não encontrado")
		return
	}
	if name == me.Username {
		writeErr(w, 400, "você não pode excluir a si mesmo")
		return
	}
	_, _ = a.exec(r.Context(), `DELETE FROM users WHERE id=?`, id)
	a.audit(r.Context(), me.Username, "user.delete", name)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := a.query(r.Context(), `SELECT at,username,action,detail FROM audit_log ORDER BY at DESC LIMIT 200`)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	defer rows.Close()
	type row struct {
		At       time.Time `json:"at"`
		Username string    `json:"username"`
		Action   string    `json:"action"`
		Detail   string    `json:"detail"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if rows.Scan(&x.At, &x.Username, &x.Action, &x.Detail) == nil {
			out = append(out, x)
		}
	}
	writeJSON(w, 200, out)
}

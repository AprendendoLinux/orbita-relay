package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		if isHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/public/branding", a.handleBranding)
	mux.HandleFunc("POST /api/setup/db", a.handleSetupDB)
	mux.HandleFunc("POST /api/setup/admin", a.handleSetupAdmin)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)

	mux.HandleFunc("GET /api/me", a.auth("viewer", a.handleMe))
	mux.HandleFunc("POST /api/me/password", a.auth("viewer", a.handleChangePassword))
	mux.HandleFunc("GET /api/stats", a.auth("viewer", a.handleStats))
	mux.HandleFunc("GET /api/messages", a.auth("viewer", a.handleMessages))
	mux.HandleFunc("GET /api/messages/export", a.auth("viewer", a.handleExport))
	mux.HandleFunc("GET /api/messages/{id}", a.auth("viewer", a.handleMessageDetail))

	mux.HandleFunc("GET /api/admin/settings", a.auth("admin", a.handleGetSettings))
	mux.HandleFunc("PUT /api/admin/settings", a.auth("admin", a.handlePutSettings))
	mux.HandleFunc("POST /api/admin/ldap/test", a.auth("admin", func(w http.ResponseWriter, r *http.Request) {
		if err := a.testLDAP(); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	}))
	mux.HandleFunc("GET /api/admin/users", a.auth("admin", a.handleListUsers))
	mux.HandleFunc("POST /api/admin/users", a.auth("admin", a.handleCreateUser))
	mux.HandleFunc("PATCH /api/admin/users/{id}", a.auth("admin", a.handleUpdateUser))
	mux.HandleFunc("DELETE /api/admin/users/{id}", a.auth("admin", a.handleDeleteUser))
	mux.HandleFunc("GET /api/admin/audit", a.auth("admin", a.handleAudit))

	mux.HandleFunc("GET /api/admin/themes", a.auth("admin", a.handleListThemes))
	mux.HandleFunc("POST /api/admin/themes", a.auth("admin", a.handleCreateTheme))
	mux.HandleFunc("PUT /api/admin/themes/{slug}", a.auth("admin", a.handleUpdateTheme))
	mux.HandleFunc("DELETE /api/admin/themes/{slug}", a.auth("admin", a.handleDeleteTheme))
	mux.HandleFunc("POST /api/admin/themes/{slug}/activate", a.auth("admin", a.handleActivateTheme))
	mux.HandleFunc("POST /api/admin/themes/{slug}/logo", a.auth("admin", a.handleThemeLogo))
	mux.HandleFunc("DELETE /api/admin/themes/{slug}/logo", a.auth("admin", a.handleDeleteThemeLogo))

	mux.HandleFunc("GET /api/admin/domains", a.auth("admin", a.handleListDomains))
	mux.HandleFunc("POST /api/admin/domains", a.auth("admin", a.handleCreateDomain))
	mux.HandleFunc("DELETE /api/admin/domains/{id}", a.auth("admin", a.handleDeleteDomain))
	mux.HandleFunc("POST /api/admin/domains/{id}/dkim", a.auth("admin", a.handleGenerateDKIM))
	mux.HandleFunc("PUT /api/admin/domains/{id}/dkim", a.auth("admin", a.handleToggleDKIM))
	mux.HandleFunc("GET /api/admin/domains/{id}/check", a.auth("admin", a.handleCheckDomain))

	mux.HandleFunc("GET /uploads/{name}", a.handleUploads)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{"ok": true, "version": version}) })
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) { writeErr(w, 404, "não encontrado") })
	mux.HandleFunc("GET /", a.serveSPA)
	return securityHeaders(mux)
}

func (a *App) serveSPA(w http.ResponseWriter, r *http.Request) {
	clean := filepath.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
	full := filepath.Join(a.webDir, clean)
	if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
		if strings.HasPrefix(clean, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, full)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, filepath.Join(a.webDir, "index.html"))
}

func (a *App) runMaintenance(ctx context.Context) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		if a.ready() {
			days := a.sInt("db_retention_days")
			if days > 0 {
				if _, err := a.db.Exec(ctx, `DELETE FROM messages WHERE received_at < now() - make_interval(days => $1)`, days); err != nil {
					log.Printf("retenção: %v", err)
				}
			}
			_, _ = a.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
			_, _ = a.db.Exec(ctx, `DELETE FROM audit_log WHERE at < now() - interval '1 year'`)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

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

	mux.HandleFunc("POST /api/collect", a.handleCollect)
	mux.HandleFunc("GET /api/collect/ping", a.handleCollectPing)

	mux.HandleFunc("GET /api/auth/confirm-email", a.handleEmailConfirmPreview)
	mux.HandleFunc("POST /api/auth/confirm-email", a.handleEmailConfirm)
	mux.HandleFunc("GET /api/me/email", a.auth("viewer", a.handleEmailChangeStatus))
	mux.HandleFunc("POST /api/me/email/change", a.auth("viewer", a.handleEmailChangeRequest))
	mux.HandleFunc("DELETE /api/me/email/change", a.auth("viewer", a.handleEmailChangeCancel))
	mux.HandleFunc("GET /api/auth/reset", a.handleResetCheck)
	mux.HandleFunc("POST /api/auth/forgot", a.handleForgot)
	mux.HandleFunc("POST /api/auth/reset", a.handleReset)
	mux.HandleFunc("POST /api/auth/mfa", a.handleLoginMFA)
	mux.HandleFunc("POST /api/auth/mfa/resend", a.handleLoginMFAResend)
	mux.HandleFunc("GET /api/me/mfa", a.auth("viewer", a.handleMFAStatus))
	mux.HandleFunc("POST /api/me/mfa/totp/setup", a.auth("viewer", a.handleTOTPSetup))
	mux.HandleFunc("POST /api/me/mfa/totp/enable", a.auth("viewer", a.handleTOTPEnable))
	mux.HandleFunc("POST /api/me/mfa/email/setup", a.auth("viewer", a.handleEmailSetup))
	mux.HandleFunc("POST /api/me/mfa/email/enable", a.auth("viewer", a.handleEmailEnable))
	mux.HandleFunc("POST /api/me/mfa/disable", a.auth("viewer", a.handleMFADisable))
	mux.HandleFunc("POST /api/me/mfa/trusted/revoke", a.auth("viewer", a.handleTrustedRevoke))
	mux.HandleFunc("POST /api/me/mfa/recovery", a.auth("viewer", a.handleRecoveryRegen))
	mux.HandleFunc("POST /api/admin/users/{id}/mfa/reset", a.auth("admin", a.handleAdminMFAReset))
	mux.HandleFunc("POST /api/admin/networks/detect", a.auth("admin", a.handleDetectNets))
	mux.HandleFunc("POST /api/admin/smtp/test", a.auth("admin", a.handleTestSMTP))
	mux.HandleFunc("GET /api/me", a.auth("viewer", a.handleMe))
	mux.HandleFunc("POST /api/me/password", a.auth("viewer", a.handleChangePassword))
	mux.HandleFunc("GET /api/stats", a.auth("viewer", a.handleStats))
	mux.HandleFunc("GET /api/messages", a.auth("viewer", a.handleMessages))
	mux.HandleFunc("GET /api/messages/export", a.auth("viewer", a.handleExport))
	mux.HandleFunc("GET /api/messages/report.pdf", a.auth("viewer", a.handleReportPDF))
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

	mux.HandleFunc("GET /api/nodes", a.auth("viewer", a.handleNodeNames))
	mux.HandleFunc("GET /api/admin/nodes", a.auth("admin", a.handleListNodes))
	mux.HandleFunc("POST /api/admin/nodes", a.auth("admin", a.handleCreateNode))
	mux.HandleFunc("PATCH /api/admin/nodes/{id}", a.auth("admin", a.handleUpdateNode))
	mux.HandleFunc("POST /api/admin/nodes/{id}/token", a.auth("admin", a.handleRotateNode))
	mux.HandleFunc("DELETE /api/admin/nodes/{id}", a.auth("admin", a.handleDeleteNode))
	mux.HandleFunc("GET /api/admin/mailcow", a.auth("admin", a.handleListMailcow))
	mux.HandleFunc("POST /api/admin/mailcow", a.auth("admin", a.handleCreateMailcow))
	mux.HandleFunc("PUT /api/admin/mailcow/{id}", a.auth("admin", a.handleEditMailcow))
	mux.HandleFunc("POST /api/admin/mailcow/test", a.auth("admin", a.handleTestMailcow))
	mux.HandleFunc("POST /api/admin/collect/test", a.auth("admin", a.handleCollectTest))
	mux.HandleFunc("GET /api/admin/ipv6", a.auth("admin", a.handleIPv6Status))
	mux.HandleFunc("GET /api/admin/collect/status", a.auth("admin", a.handleCollectStatus))

	mux.HandleFunc("GET /api/admin/domains", a.auth("admin", a.handleListDomains))
	mux.HandleFunc("POST /api/admin/domains", a.auth("admin", a.handleCreateDomain))
	mux.HandleFunc("DELETE /api/admin/domains/{id}", a.auth("admin", a.handleDeleteDomain))
	mux.HandleFunc("POST /api/admin/domains/{id}/dkim", a.auth("admin", a.handleGenerateDKIM))
	mux.HandleFunc("PUT /api/admin/domains/{id}/dkim", a.auth("admin", a.handleToggleDKIM))
	mux.HandleFunc("GET /api/admin/domains/{id}/check", a.auth("admin", a.handleCheckDomain))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "version": version})
	})
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
				if _, err := a.exec(ctx, `DELETE FROM messages WHERE received_at < DATE_SUB(UTC_TIMESTAMP(6), INTERVAL ? DAY)`, days); err != nil {
					log.Printf("retenção: %v", err)
				}
			}
			_, _ = a.exec(ctx, `DELETE FROM sessions WHERE expires_at < UTC_TIMESTAMP(6)`)
			_, _ = a.exec(ctx, `DELETE FROM audit_log WHERE at < DATE_SUB(UTC_TIMESTAMP(6), INTERVAL 1 YEAR)`)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

type Domain struct {
	ID          int64     `json:"id"`
	Domain      string    `json:"domain"`
	DKIMEnabled bool      `json:"dkim_enabled"`
	DKIMRecord  string    `json:"dkim_record,omitempty"`
	DKIMHost    string    `json:"dkim_host,omitempty"`
	SPFRecord   string    `json:"spf_record"`
	DMARCHost   string    `json:"dmarc_host"`
	DMARCRecord string    `json:"dmarc_record"`
	CreatedAt   time.Time `json:"created_at"`
}

func (a *App) handleListDomains(w http.ResponseWriter, r *http.Request) {
	rows, err := a.query(r.Context(), `SELECT id,domain,dkim_enabled,dkim_public,created_at FROM domains ORDER BY domain`)
	if err != nil {
		writeErr(w, 500, "erro de banco")
		return
	}
	defer rows.Close()
	sel := a.sStr("dkim_selector")
	out := []Domain{}
	for rows.Next() {
		var d Domain
		var pub string
		if rows.Scan(&d.ID, &d.Domain, &d.DKIMEnabled, &pub, &d.CreatedAt) != nil {
			continue
		}
		if pub != "" {
			d.DKIMHost = sel + "._domainkey." + d.Domain
			d.DKIMRecord = "v=DKIM1; k=rsa; p=" + pub
		}
		d.DMARCHost = "_dmarc." + d.Domain
		d.SPFRecord = a.spfRecord()
		d.DMARCRecord = a.dmarcRecord()
		out = append(out, d)
	}
	writeJSON(w, 200, out)
}

func (a *App) handleCreateDomain(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Domain string `json:"domain"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	d := strings.ToLower(strings.TrimSpace(in.Domain))
	if !reDomain.MatchString(d) {
		writeErr(w, 400, "domínio inválido")
		return
	}
	if _, err := a.exec(r.Context(), `INSERT INTO domains(domain) VALUES(?)`, d); err != nil {
		writeErr(w, 409, "domínio já cadastrado")
		return
	}
	a.audit(r.Context(), userFrom(r).Username, "domain.create", d)
	_ = a.renderMTA(r.Context())
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (a *App) handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	var d string
	if err := a.row(r.Context(), `SELECT domain FROM domains WHERE id=?`, r.PathValue("id")).Scan(&d); err != nil {
		writeErr(w, 404, "domínio não encontrado")
		return
	}
	_, _ = a.exec(r.Context(), `DELETE FROM domains WHERE id=?`, r.PathValue("id"))
	a.audit(r.Context(), userFrom(r).Username, "domain.delete", d)
	_ = a.renderMTA(r.Context())
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleGenerateDKIM(w http.ResponseWriter, r *http.Request) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		writeErr(w, 500, "falha ao gerar chave")
		return
	}
	priv := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	pubDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	enc, err := a.encrypt(priv)
	if err != nil {
		writeErr(w, 500, "falha ao criptografar chave")
		return
	}
	var d string
	id := r.PathValue("id")
	_, _ = a.exec(r.Context(), `UPDATE domains SET dkim_private=?, dkim_public=?, dkim_enabled=1 WHERE id=?`,
		enc, base64.StdEncoding.EncodeToString(pubDER), id)
	if err = a.row(r.Context(), `SELECT domain FROM domains WHERE id=?`, id).Scan(&d); err != nil {
		writeErr(w, 404, "domínio não encontrado")
		return
	}
	a.audit(r.Context(), userFrom(r).Username, "dkim.generate", d)
	_ = a.renderMTA(r.Context())
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *App) handleToggleDKIM(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	res, err := a.exec(r.Context(), `UPDATE domains SET dkim_enabled=? WHERE id=? AND dkim_private<>''`, in.Enabled, r.PathValue("id"))
	if n, _ := res.RowsAffected(); err != nil || n == 0 {
		writeErr(w, 400, "gere a chave DKIM primeiro")
		return
	}
	_ = a.renderMTA(r.Context())
	writeJSON(w, 200, map[string]any{"ok": true})
}

func lookupTXT(ctx context.Context, name string) []string {
	res := &net.Resolver{}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	txt, err := res.LookupTXT(cctx, name)
	if err != nil {
		return nil
	}
	return txt
}

func (a *App) handleCheckDomain(w http.ResponseWriter, r *http.Request) {
	var d, pub string
	if err := a.row(r.Context(), `SELECT domain,dkim_public FROM domains WHERE id=?`, r.PathValue("id")).Scan(&d, &pub); err != nil {
		writeErr(w, 404, "domínio não encontrado")
		return
	}
	type check struct {
		OK     bool     `json:"ok"`
		Found  []string `json:"found"`
		Detail string   `json:"detail"`
	}
	var spf, dmarc, dkim check
	for _, t := range lookupTXT(r.Context(), d) {
		if strings.HasPrefix(strings.ToLower(t), "v=spf1") {
			spf.Found = append(spf.Found, t)
			spf.OK = true
		}
	}
	if !spf.OK {
		spf.Detail = "nenhum registro SPF encontrado"
	}
	for _, t := range lookupTXT(r.Context(), "_dmarc."+d) {
		if strings.HasPrefix(strings.ToLower(t), "v=dmarc1") {
			dmarc.Found = append(dmarc.Found, t)
			dmarc.OK = true
		}
	}
	if !dmarc.OK {
		dmarc.Detail = "nenhum registro DMARC encontrado"
	}
	if pub == "" {
		dkim.Detail = "chave DKIM não gerada"
	} else {
		for _, t := range lookupTXT(r.Context(), a.sStr("dkim_selector")+"._domainkey."+d) {
			dkim.Found = append(dkim.Found, t)
			if strings.Contains(strings.ReplaceAll(t, " ", ""), pub) {
				dkim.OK = true
			}
		}
		if !dkim.OK {
			dkim.Detail = "registro ausente ou diferente da chave gerada"
		}
	}
	writeJSON(w, 200, map[string]any{"spf": spf, "dmarc": dmarc, "dkim": dkim})
}

func (a *App) spfRecord() string {
	var parts []string
	for _, l := range strings.Split(a.sStr("public_ips"), "\n") {
		l = strings.TrimSpace(l)
		if ip, err := netip.ParseAddr(l); err == nil {
			if ip.Is4() {
				parts = append(parts, "ip4:"+l)
			} else {
				parts = append(parts, "ip6:"+l)
			}
		}
	}
	if len(parts) == 0 {
		if h := a.sStr("mta_hostname"); h != "" {
			parts = append(parts, "a:"+h)
		} else {
			parts = append(parts, "ip4:IP_DO_SERVIDOR")
		}
	}
	return "v=spf1 " + strings.Join(parts, " ") + " ~all"
}

func (a *App) dmarcRecord() string {
	rec := "v=DMARC1; p=" + a.sStr("dmarc_policy")
	if rua := a.sStr("dmarc_rua"); rua != "" {
		rec += "; rua=mailto:" + rua
	}
	return rec + "; adkim=r; aspf=r; pct=100"
}

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
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
	SPFNote     string    `json:"spf_note"`
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
		d.SPFRecord, d.SPFNote = a.spfRecord(r.Context(), d.Domain)
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
	res, err := a.exec(r.Context(), `INSERT INTO domains(domain) VALUES(?)`, d)
	if err != nil {
		writeErr(w, 409, "domínio já cadastrado")
		return
	}
	id, _ := res.LastInsertId()
	if err := a.makeDKIM(r.Context(), id); err != nil {
		_, _ = a.exec(r.Context(), `DELETE FROM domains WHERE id=?`, id)
		writeErr(w, 500, err.Error())
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

func (a *App) makeDKIM(ctx context.Context, id any) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return errors.New("falha ao gerar chave")
	}
	priv := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	pubDER, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	enc, err := a.encrypt(priv)
	if err != nil {
		return errors.New("falha ao criptografar chave")
	}
	_, err = a.exec(ctx, `UPDATE domains SET dkim_private=?, dkim_public=?, dkim_enabled=1 WHERE id=?`,
		enc, base64.StdEncoding.EncodeToString(pubDER), id)
	return err
}

func (a *App) handleGenerateDKIM(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var d string
	if err := a.row(r.Context(), `SELECT domain FROM domains WHERE id=?`, id).Scan(&d); err != nil {
		writeErr(w, 404, "domínio não encontrado")
		return
	}
	if err := a.makeDKIM(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
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

func dnsServers() []string {
	raw := strings.FieldsFunc(os.Getenv("ORBITA_DNS_SERVERS"), func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	if len(raw) == 0 {
		raw = []string{"8.8.8.8", "1.1.1.1"}
	}
	var out []string
	for _, s := range raw {
		if _, _, err := net.SplitHostPort(s); err != nil {
			if strings.Contains(s, ":") {
				s = "[" + s + "]"
			}
			s += ":53"
		}
		out = append(out, s)
	}
	return out
}

func publicResolver(server string) *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		d := net.Dialer{Timeout: 3 * time.Second}
		return d.DialContext(ctx, network, server)
	}}
}

func resolveWith(ctx context.Context, f func(context.Context, *net.Resolver) ([]string, error)) ([]string, bool) {
	for _, s := range dnsServers() {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		out, err := f(cctx, publicResolver(s))
		cancel()
		if err == nil {
			return out, true
		}
		var de *net.DNSError
		if errors.As(err, &de) && de.IsNotFound {
			return nil, true
		}
	}
	return nil, false
}

func lookupTXT(ctx context.Context, name string) []string {
	if out, ok := resolveWith(ctx, func(c context.Context, r *net.Resolver) ([]string, error) { return r.LookupTXT(c, name) }); ok {
		return out
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	txt, err := (&net.Resolver{}).LookupTXT(cctx, name)
	if err != nil {
		return nil
	}
	return txt
}

func lookupHostIPs(ctx context.Context, name string) []netip.Addr {
	var out []netip.Addr
	add := func(ss []string) {
		for _, s := range ss {
			if ip, err := netip.ParseAddr(s); err == nil {
				out = append(out, ip.Unmap())
			}
		}
	}
	if ss, ok := resolveWith(ctx, func(c context.Context, r *net.Resolver) ([]string, error) { return r.LookupHost(c, name) }); ok {
		add(ss)
		return out
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if ss, err := (&net.Resolver{}).LookupHost(cctx, name); err == nil {
		add(ss)
	}
	return out
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

func (a *App) spfRecord(ctx context.Context, domain string) (string, string) {
	// O SPF autoriza o IP que o hostname do relay (HELO) aponta no DNS. Sem hostname,
	// usa o próprio domínio de remetente.
	if host := a.sStr("mta_hostname"); host != "" {
		return "v=spf1 a:" + host + " ~all", "Usa o IP do hostname do relay (" + host + "), detectado pelo DNS."
	}
	note := "Usa o IP do próprio domínio, detectado pelo DNS."
	if len(lookupHostIPs(ctx, domain)) == 0 {
		note = "Atenção: " + domain + " ainda não tem registro A/AAAA no DNS. Informe o hostname do relay em Configurações > Geral."
	}
	return "v=spf1 a:" + domain + " ~all", note
}

func (a *App) dmarcRecord() string {
	rec := "v=DMARC1; p=" + a.sStr("dmarc_policy")
	if rua := a.sStr("dmarc_rua"); rua != "" {
		rec += "; rua=mailto:" + rua
	}
	return rec + "; adkim=r; aspf=r; pct=100"
}

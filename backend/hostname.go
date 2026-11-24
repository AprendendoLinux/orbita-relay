package main

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// Hostname do relay (modo Relay SMTP): o painel consulta o A/AAAA e o PTR, e só
// autoriza a enviar os IPs que o administrador selecionar. Os IPs autorizados ficam
// registrados (hostname_tracked_*), para que sejam removidos se o hostname mudar.
const (
	trackedHostKey = "hostname_tracked_host"
	trackedIPsKey  = "hostname_tracked_ips"
)

var reHostname = regexp.MustCompile(`(?i)^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

type hostnameCheck struct {
	Hostname   string            `json:"hostname"`
	OK         bool              `json:"ok"`
	IPs        []string          `json:"ips"`
	PTR        map[string]string `json:"ptr"`
	Tracked    []string          `json:"tracked"`
	Cloudflare []string          `json:"cloudflare"`
	CFSource   string            `json:"cf_source"`
	CFStale    bool              `json:"cf_stale"`
	CFUpdated  string            `json:"cf_updated,omitempty"`
}

// suggestedRelayHostname sugere o nome pelo qual o painel foi acessado, mas só se ele
// não estiver atrás do proxy da Cloudflare (nesse caso o nome não serve para o relay).
func suggestedRelayHostname(r *http.Request) string {
	name := hostnameFromRequest(r)
	if name == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	for _, ip := range resolveRelayHost(ctx, name) {
		if isCloudflareIP(ip) {
			return ""
		}
	}
	return name
}

func cleanHostname(s string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
}

// resolveRelayHost devolve os IPs (A/AAAA) do hostname, sem repetição e em ordem estável.
func resolveRelayHost(ctx context.Context, host string) []netip.Addr {
	seen := map[netip.Addr]bool{}
	var out []netip.Addr
	for _, ip := range lookupHostIPs(ctx, host) {
		ip = ip.Unmap()
		if !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() || seen[ip] {
			continue
		}
		seen[ip] = true
		out = append(out, ip)
	}
	return out
}

func (a *App) trackedFor(host string) []string {
	if !strings.EqualFold(a.sStr(trackedHostKey), host) {
		return nil
	}
	return splitLines(a.sStr(trackedIPsKey))
}

func (a *App) handleCheckHostname(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hostname string `json:"hostname"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	host := cleanHostname(in.Hostname)
	if !reHostname.MatchString(host) {
		writeErr(w, 400, "hostname inválido: use um nome como relay.seudominio.com.br")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	res := hostnameCheck{Hostname: host, PTR: map[string]string{}, IPs: []string{}, Tracked: a.trackedFor(host), Cloudflare: []string{}}
	for _, ip := range resolveRelayHost(ctx, host) {
		res.IPs = append(res.IPs, ip.String())
		if isCloudflareIP(ip) {
			res.Cloudflare = append(res.Cloudflare, ip.String())
		}
		pctx, pcancel := context.WithTimeout(ctx, 3*time.Second)
		if names, err := net.DefaultResolver.LookupAddr(pctx, ip.String()); err == nil && len(names) > 0 {
			res.PTR[ip.String()] = strings.TrimSuffix(names[0], ".")
		}
		pcancel()
	}
	res.OK = len(res.IPs) > 0
	src, upd, stale := cloudflareStatus()
	res.CFSource, res.CFStale = src, stale
	if !upd.IsZero() {
		res.CFUpdated = upd.Format(time.RFC3339)
	}
	writeJSON(w, 200, res)
}

func (a *App) handleAuthorizeHostname(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hostname string   `json:"hostname"`
		IPs      []string `json:"ips"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	host := cleanHostname(in.Hostname)
	if !reHostname.MatchString(host) {
		writeErr(w, 400, "hostname inválido")
		return
	}
	if len(in.IPs) == 0 {
		writeErr(w, 400, "selecione ao menos um IP")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	// Só aceita IPs que o DNS devolve agora: o administrador escolhe, mas não digita IPs livres.
	allowed := map[string]bool{}
	for _, ip := range resolveRelayHost(ctx, host) {
		if isCloudflareIP(ip) {
			continue // IP da Cloudflare nunca é autorizado
		}
		allowed[ip.String()] = true
	}
	var desired []string
	seen := map[string]bool{}
	for _, s := range in.IPs {
		ip, err := netip.ParseAddr(strings.TrimSpace(s))
		if err != nil || !allowed[ip.Unmap().String()] {
			writeErr(w, 400, "IP não encontrado no DNS de "+host+": "+s)
			return
		}
		ip = ip.Unmap()
		p := netip.PrefixFrom(ip, ip.BitLen()).String() // exatamente o IP: /32 ou /128
		if !seen[p] {
			seen[p] = true
			desired = append(desired, p)
		}
	}
	prev := a.trackedFor(host)
	if old := splitLines(a.sStr(trackedIPsKey)); !strings.EqualFold(a.sStr(trackedHostKey), host) {
		prev = append(prev, old...) // IPs de um hostname anterior também saem
	}
	out := applyTrackedNets(splitLines(a.sStr("relay_nets")), prev, desired)
	if err := a.setRaw(r.Context(), trackedHostKey, host); err != nil {
		writeErr(w, 500, "falha ao salvar")
		return
	}
	if err := a.setRaw(r.Context(), trackedIPsKey, strings.Join(desired, "\n")); err != nil {
		writeErr(w, 500, "falha ao salvar")
		return
	}
	if err := a.setRaw(r.Context(), "relay_nets", strings.Join(out, "\n")); err != nil {
		writeErr(w, 500, "falha ao salvar")
		return
	}
	if err := a.renderMTA(r.Context()); err != nil {
		writeErr(w, 500, "IPs autorizados, mas falha ao gerar config do MTA: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"relay_nets": strings.Join(out, "\n"), "tracked": desired})
}

// applyTrackedNets troca os IPs antigos autorizados pelo hostname (prev) pelos novos (desired).
// Entradas digitadas à mão, que não estão em prev, não são mexidas.
func applyTrackedNets(cur, prev, desired []string) []string {
	inPrev := toSet(prev)
	inDesired := toSet(desired)
	var out []string
	inOut := map[string]bool{}
	for _, c := range cur {
		if inPrev[c] && !inDesired[c] {
			continue
		}
		if !inOut[c] {
			inOut[c] = true
			out = append(out, c)
		}
	}
	for _, d := range desired {
		if !inOut[d] {
			inOut[d] = true
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		out = []string{"127.0.0.1", "::1"}
	}
	return out
}

// dropStaleHostnameNets remove da lista os IPs autorizados por um hostname que não é mais o do relay.
// Não adiciona nada: o novo hostname precisa ser confirmado de novo.
func (a *App) dropStaleHostnameNets(ctx context.Context) error {
	tracked := a.sStr(trackedHostKey)
	if tracked == "" || strings.EqualFold(tracked, cleanHostname(a.sStr("mta_hostname"))) {
		return nil
	}
	prev := splitLines(a.sStr(trackedIPsKey))
	cur := splitLines(a.sStr("relay_nets"))
	out := applyTrackedNets(cur, prev, nil)
	if err := a.setRaw(ctx, "relay_nets", strings.Join(out, "\n")); err != nil {
		return err
	}
	if err := a.setRaw(ctx, trackedHostKey, ""); err != nil {
		return err
	}
	return a.setRaw(ctx, trackedIPsKey, "")
}

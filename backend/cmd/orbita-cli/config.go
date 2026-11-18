package main

import (
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Mode          string
	Hostname      string
	IPv4Only      bool
	MaxMB         int
	RelayNets     []string
	RelayNetsAuto bool
	HostIPs       []string
	Domains       []string
	Selector      string
	Enforce       bool
	PublicIPs     []string
	DMARCPolicy   string
	DMARCRua      string

	SHHost string
	SHPort int
	SHTLS  string
	SHUser string
	SHPass string

	CentralScheme string
	CentralHost   string
	CentralPort   int
	CentralPath   string
	CentralToken  string
	CentralSkip   bool
	CentralEvery  time.Duration

	LogRotateMB int
	LogKeep     int
	DataDir     string
}

var (
	reHost     = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$`)
	reSelector = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,30}[a-zA-Z0-9])?$`)
)

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' })
}

func parseBool(s string, def bool) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return def, true
	case "1", "true", "yes", "sim", "on":
		return true, true
	case "0", "false", "no", "nao", "não", "off":
		return false, true
	}
	return def, false
}

func validNet(s string) (string, bool) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked().String(), true
	}
	if ip, err := netip.ParseAddr(s); err == nil {
		return ip.Unmap().String(), true
	}
	return "", false
}

func loadConfig(get func(string) string) (Config, []string) {
	var errs []string
	bad := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	str := func(k, def string) string {
		if v := strings.TrimSpace(get(k)); v != "" {
			return v
		}
		return def
	}
	num := func(k string, def, lo, hi int) int {
		v := strings.TrimSpace(get(k))
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < lo || n > hi {
			bad("%s: valor inválido %q (use um número entre %d e %d)", k, v, lo, hi)
			return def
		}
		return n
	}
	flag := func(k string, def bool) bool {
		b, ok := parseBool(get(k), def)
		if !ok {
			bad("%s: valor inválido %q (use true ou false)", k, get(k))
		}
		return b
	}

	c := Config{DataDir: str("ORBITA_DATA_DIR", "/data")}
	c.Mode = strings.ToLower(str("ORBITA_MODE", "relay"))
	if c.Mode != "relay" && c.Mode != "smarthost" {
		bad("ORBITA_MODE: use relay ou smarthost (recebido %q)", c.Mode)
	}
	c.Hostname = strings.ToLower(str("ORBITA_HOSTNAME", ""))
	switch {
	case c.Hostname == "":
		bad("ORBITA_HOSTNAME é obrigatória: informe o nome DNS do relay (ex.: relay.exemplo.com.br)")
	case !reHost.MatchString(c.Hostname):
		bad("ORBITA_HOSTNAME: %q não é um nome DNS válido", c.Hostname)
	}
	switch m := strings.ToLower(str("ORBITA_IP_MODE", "dual")); m {
	case "dual":
	case "ipv4":
		c.IPv4Only = true
	default:
		bad("ORBITA_IP_MODE: use dual ou ipv4 (recebido %q)", m)
	}
	c.MaxMB = num("ORBITA_MAX_MESSAGE_MB", 25, 1, 1024)

	nets := str("ORBITA_RELAY_NETS", "10.0.0.0/8,192.168.0.0/16,fd00::/8")
	for _, n := range splitList(nets) {
		if v, ok := validNet(n); ok {
			c.RelayNets = append(c.RelayNets, v)
		} else {
			bad("ORBITA_RELAY_NETS: %q não é um IP nem uma rede CIDR válida", n)
		}
	}
	c.RelayNetsAuto = flag("ORBITA_RELAY_NETS_AUTO", true)
	for _, n := range splitList(get("ORBITA_HOST_IPS")) {
		if v, ok := validNet(n); ok {
			c.HostIPs = append(c.HostIPs, v)
		} else {
			bad("ORBITA_HOST_IPS: %q não é um IP nem uma rede CIDR válida", n)
		}
	}

	seen := map[string]bool{}
	for _, d := range splitList(get("ORBITA_DOMAINS")) {
		d = strings.ToLower(strings.TrimSuffix(d, "."))
		if !reHost.MatchString(d) {
			bad("ORBITA_DOMAINS: %q não é um domínio válido", d)
			continue
		}
		if !seen[d] {
			seen[d] = true
			c.Domains = append(c.Domains, d)
		}
	}
	c.Selector = str("ORBITA_DKIM_SELECTOR", "dkim")
	if !reSelector.MatchString(c.Selector) {
		bad("ORBITA_DKIM_SELECTOR: %q inválido (letras, números e hífen)", c.Selector)
	}
	c.Enforce = flag("ORBITA_ENFORCE_SENDER_DOMAINS", true)
	if c.Enforce && len(c.Domains) == 0 && c.Mode == "relay" {
		c.Enforce = false
	}

	for _, ip := range splitList(get("ORBITA_PUBLIC_IPS")) {
		if a, err := netip.ParseAddr(ip); err == nil {
			c.PublicIPs = append(c.PublicIPs, a.Unmap().String())
		} else {
			bad("ORBITA_PUBLIC_IPS: %q não é um IP válido", ip)
		}
	}
	c.DMARCPolicy = strings.ToLower(str("ORBITA_DMARC_POLICY", "none"))
	if c.DMARCPolicy != "none" && c.DMARCPolicy != "quarantine" && c.DMARCPolicy != "reject" {
		bad("ORBITA_DMARC_POLICY: use none, quarantine ou reject")
	}
	c.DMARCRua = str("ORBITA_DMARC_RUA", "")

	c.SHHost = str("ORBITA_SMARTHOST_HOST", "")
	c.SHPort = num("ORBITA_SMARTHOST_PORT", 587, 1, 65535)
	c.SHTLS = strings.ToLower(str("ORBITA_SMARTHOST_TLS", "starttls"))
	if c.SHTLS != "starttls" && c.SHTLS != "tls" && c.SHTLS != "none" {
		bad("ORBITA_SMARTHOST_TLS: use starttls, tls ou none")
	}
	c.SHUser = str("ORBITA_SMARTHOST_USER", "")
	c.SHPass = get("ORBITA_SMARTHOST_PASS")
	if c.Mode == "smarthost" {
		if c.SHHost == "" {
			bad("ORBITA_SMARTHOST_HOST é obrigatória no modo smarthost")
		}
		if (c.SHUser == "") != (c.SHPass == "") {
			bad("ORBITA_SMARTHOST_USER e ORBITA_SMARTHOST_PASS devem ser informados juntos")
		}
	}

	if raw := str("ORBITA_CENTRAL_URL", ""); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			bad("ORBITA_CENTRAL_URL: %q inválida (ex.: https://orbita.exemplo.com.br)", raw)
		} else {
			c.CentralScheme, c.CentralHost = u.Scheme, u.Hostname()
			c.CentralPath = strings.TrimRight(u.Path, "/")
			if p := u.Port(); p != "" {
				c.CentralPort, _ = strconv.Atoi(p)
			} else if u.Scheme == "http" {
				c.CentralPort = 80
			} else {
				c.CentralPort = 443
			}
		}
		c.CentralToken = str("ORBITA_CENTRAL_TOKEN", "")
		if c.CentralToken == "" {
			bad("ORBITA_CENTRAL_TOKEN é obrigatória quando ORBITA_CENTRAL_URL é informada")
		}
		c.CentralSkip = flag("ORBITA_CENTRAL_SKIP_VERIFY", false)
		c.CentralEvery = time.Duration(num("ORBITA_CENTRAL_INTERVAL", 10, 2, 3600)) * time.Second
	}
	c.LogRotateMB = num("ORBITA_LOG_ROTATE_MB", 50, 1, 10240)
	c.LogKeep = num("ORBITA_LOG_KEEP", 7, 1, 365)
	return c, errs
}

func (c Config) centralOn() bool { return c.CentralHost != "" }

func (c Config) effectiveNets() []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, n := range c.RelayNets {
		add(n)
	}
	if c.RelayNetsAuto {
		add("127.0.0.1")
		add("::1")
		for _, n := range ifaceNets() {
			add(n)
		}
		for _, n := range c.HostIPs {
			add(n)
		}
	}
	if len(out) == 0 {
		out = []string{"127.0.0.1", "::1"}
	}
	return out
}

func (c Config) spfRecord() string {
	var parts []string
	for _, ip := range c.PublicIPs {
		if a, err := netip.ParseAddr(ip); err == nil && a.Is6() {
			parts = append(parts, "ip6:"+ip)
		} else {
			parts = append(parts, "ip4:"+ip)
		}
	}
	if len(parts) == 0 {
		parts = []string{"a:" + c.Hostname}
	}
	return "v=spf1 " + strings.Join(parts, " ") + " ~all"
}

func (c Config) dmarcRecord() string {
	rec := "v=DMARC1; p=" + c.DMARCPolicy
	if c.DMARCRua != "" {
		rec += "; rua=mailto:" + c.DMARCRua
	}
	return rec + "; adkim=r; aspf=r; pct=100"
}

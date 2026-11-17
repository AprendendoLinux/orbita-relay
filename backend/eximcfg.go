package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

type eximView struct {
	Hostname      string
	IPv4Only      bool
	RelayNets     string
	MaxMB         int
	SenderDomains string
	DKIMDomains   string
	Selector      string
	Smarthost     bool
	SmarthostHost string
	Port          int
	TLS           string
	HasAuth       bool
	User          string
	Pass          string
}

const eximTemplate = `# Gerado automaticamente pelo OrbitaRelay. Não edite manualmente.
{{if .Hostname}}primary_hostname = {{.Hostname}}
{{end}}spool_directory = /var/spool/exim4
pid_file_path = /run/exim4/exim.pid
log_file_path = /var/log/exim/%s.log
log_selector = +subject +smtp_confirmation +tls_cipher +delivery_size +sender_on_delivery
log_timezone = true
message_logs = false
host_lookup = *
rfc1413_query_timeout = 0s
never_users = root
{{if .IPv4Only}}disable_ipv6 = true
local_interfaces = <; 0.0.0.0
{{else}}local_interfaces = <; 0.0.0.0 ; ::0
{{end}}daemon_smtp_ports = 25 : 587
domainlist local_domains = @
hostlist relay_from_hosts = <; {{.RelayNets}}
acl_smtp_mail = acl_check_mail
acl_smtp_rcpt = acl_check_rcpt
message_size_limit = {{.MaxMB}}M
smtp_accept_max = 200
smtp_accept_max_per_host = 50
smtp_banner = $smtp_active_hostname ESMTP OrbitaRelay
ignore_bounce_errors_after = 2d
timeout_frozen_after = 7d
tls_advertise_hosts = *
tls_certificate = /etc/exim4/tls/cert.pem
tls_privatekey = /etc/exim4/tls/key.pem

begin acl

acl_check_mail:
  deny    message = Relay not permitted
          !hosts = +relay_from_hosts
  accept  senders = :
{{if .SenderDomains}}  deny    message = Sender domain not allowed: $sender_address_domain
          condition = ${if inlist{${lc:$sender_address_domain}}{<; {{.SenderDomains}}}{no}{yes}}
{{end}}  accept

acl_check_rcpt:
  accept  hosts = +relay_from_hosts
  deny    message = Relay not permitted

begin routers

{{if .Smarthost}}smarthost:
  driver = manualroute
  domains = ! +local_domains
  transport = remote_smtp_smarthost
  route_list = * {{.SmarthostHost}} byname
  no_more
{{else}}dnslookup:
  driver = dnslookup
  domains = ! +local_domains
  transport = remote_smtp
  ignore_target_hosts = <; 0.0.0.0 ; 127.0.0.0/8 ; ::1
  no_more
{{end}}
begin transports

remote_smtp:
  driver = smtp
{{template "dkim" .}}
{{if .Smarthost}}remote_smtp_smarthost:
  driver = smtp
  port = {{.Port}}
{{if eq .TLS "tls"}}  protocol = smtps
{{end}}{{if ne .TLS "none"}}  hosts_require_tls = *
{{end}}{{if .HasAuth}}  hosts_require_auth = *
{{end}}{{template "dkim" .}}{{end}}
begin retry

* * F,2h,15m; G,16h,1h,1.5; F,4d,6h
{{if .HasAuth}}
begin authenticators

smarthost_plain:
  driver = plaintext
  public_name = PLAIN
  hide client_send = ^{{.User}}^{{.Pass}}

smarthost_login:
  driver = plaintext
  public_name = LOGIN
  hide client_send = : {{.User}} : {{.Pass}}
{{end}}`

const dkimTemplate = `{{define "dkim"}}{{if .DKIMDomains}}  dkim_domain = ${if inlist{${lc:${domain:$h_from:}}}{<; {{.DKIMDomains}}}{${lc:${domain:$h_from:}}}{}}
  dkim_selector = {{.Selector}}
  dkim_private_key = /etc/exim4/dkim/$dkim_domain.key
  dkim_canon = relaxed
{{end}}{{end}}`

var eximTmpl = template.Must(template.Must(template.New("exim").Parse(eximTemplate)).Parse(dkimTemplate))

var logrotateTmpl = template.Must(template.New("lr").Parse(`/var/log/exim/*.log {
  daily
  maxsize {{.SizeMB}}M
  rotate {{.Keep}}
  compress
  delaycompress
  missingok
  notifempty
  nomail
  create 0644 Debian-exim adm
}
`))

func escClientSend(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	return strings.ReplaceAll(s, `:`, `::`)
}

func (a *App) renderDefaultMTA() error { return a.renderMTA(context.Background()) }

func (a *App) renderMTA(ctx context.Context) error {
	v := eximView{
		Hostname: a.sStr("mta_hostname"),
		IPv4Only: a.sStr("ip_mode") == "ipv4",
		MaxMB:    max(a.sInt("max_message_mb"), 1),
		Selector: a.sStr("dkim_selector"),
		Port:     a.sInt("smarthost_port"),
		TLS:      a.sStr("smarthost_tls"),
	}
	var nets []string
	for _, l := range strings.Split(a.sStr("relay_nets"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			nets = append(nets, l)
		}
	}
	if len(nets) == 0 {
		nets = []string{"127.0.0.1", "::1"}
	}
	v.RelayNets = strings.Join(nets, " ; ")

	dkimKeys := map[string]string{}
	if a.ready() {
		rows, err := a.db.Query(ctx, `SELECT domain,dkim_enabled,dkim_private FROM domains ORDER BY domain`)
		if err != nil {
			return err
		}
		var all, signed []string
		for rows.Next() {
			var d, p string
			var en bool
			if err := rows.Scan(&d, &en, &p); err != nil {
				rows.Close()
				return err
			}
			all = append(all, d)
			if en && p != "" {
				if pem, err := a.decrypt(p); err == nil {
					dkimKeys[d] = pem
					signed = append(signed, d)
				}
			}
		}
		rows.Close()
		if a.sBool("enforce_sender_domains") {
			v.SenderDomains = strings.Join(all, " ; ")
		}
		v.DKIMDomains = strings.Join(signed, " ; ")
	}

	if a.sBool("smarthost_enabled") && a.sStr("smarthost_host") != "" {
		v.Smarthost = true
		v.SmarthostHost = a.sStr("smarthost_host")
		if v.Port == 0 {
			v.Port = 587
		}
		if u := a.sStr("smarthost_user"); u != "" && a.sStr("smarthost_pass") != "" {
			v.HasAuth = true
			v.User = escClientSend(u)
			v.Pass = escClientSend(a.sStr("smarthost_pass"))
		}
	}

	var conf, lr bytes.Buffer
	if err := eximTmpl.Execute(&conf, v); err != nil {
		return err
	}
	if err := logrotateTmpl.Execute(&lr, map[string]int{
		"SizeMB": max(a.sInt("log_rotate_mb"), 1),
		"Keep":   max(a.sInt("log_keep_files"), 1),
	}); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(a.configDir, "dkim"), 0o750); err != nil {
		return err
	}
	h := sha256.New()
	h.Write(conf.Bytes())
	h.Write(lr.Bytes())
	names := make([]string, 0, len(dkimKeys))
	for d := range dkimKeys {
		names = append(names, d)
	}
	sort.Strings(names)
	keep := map[string]bool{}
	for _, d := range names {
		fn := d + ".key"
		keep[fn] = true
		h.Write([]byte(fn + dkimKeys[d]))
		if err := writeAtomic(filepath.Join(a.configDir, "dkim", fn), []byte(dkimKeys[d]), 0o600); err != nil {
			return err
		}
	}
	if ents, err := os.ReadDir(filepath.Join(a.configDir, "dkim")); err == nil {
		for _, e := range ents {
			if !keep[e.Name()] {
				_ = os.Remove(filepath.Join(a.configDir, "dkim", e.Name()))
			}
		}
	}
	if err := writeAtomic(filepath.Join(a.configDir, "exim.conf"), conf.Bytes(), 0o600); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(a.configDir, "logrotate.conf"), lr.Bytes(), 0o644); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(a.configDir, "version"), []byte(hex.EncodeToString(h.Sum(nil))), 0o644)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return fmt.Errorf("gravar %s: %w", filepath.Base(path), err)
	}
	return os.Rename(tmp, path)
}

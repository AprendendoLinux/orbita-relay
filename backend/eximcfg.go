package main

import (
	"context"
	"strings"

	"orbitarelay/internal/eximcfg"
)

func (a *App) renderDefaultMTA() error { return a.renderMTA(context.Background()) }

func (a *App) renderMTA(ctx context.Context) error {
	v := eximcfg.View{
		Hostname: a.sStr("mta_hostname"),
		IPv4Only: a.sStr("ip_mode") == "ipv4",
		MaxMB:    max(a.sInt("max_message_mb"), 1),
		Selector: a.sStr("dkim_selector"),
		Port:     a.sInt("smarthost_port"),
		TLS:      a.sStr("smarthost_tls"),
	}
	v.RelayNets = strings.Join(a.effectiveRelayNets(), " ; ")
	v.Gateways = strings.Join(dockerGateways(), " ; ")

	relayMode := a.sStr("relay_mode") != "smarthost"
	dkimKeys := map[string]string{}
	if a.ready() {
		rows, err := a.query(ctx, `SELECT domain,dkim_enabled,dkim_private FROM domains ORDER BY domain`)
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
			if relayMode && en && p != "" {
				if pem, err := a.decrypt(p); err == nil {
					dkimKeys[d] = pem
					signed = append(signed, d)
				}
			}
		}
		rows.Close()
		if relayMode {
			if a.sBool("enforce_sender_domains") {
				v.SenderDomains = strings.Join(all, " ; ")
			}
			v.DKIMDomains = strings.Join(signed, " ; ")
		}
	}

	if !relayMode && a.sStr("smarthost_host") != "" {
		v.Smarthost = true
		v.SmarthostHost = a.sStr("smarthost_host")
		if v.Port == 0 {
			v.Port = 587
		}
		if u := a.sStr("smarthost_user"); u != "" && a.sStr("smarthost_pass") != "" {
			v.HasAuth = true
			v.User = eximcfg.EscClientSend(u)
			v.Pass = eximcfg.EscClientSend(a.sStr("smarthost_pass"))
		}
	}

	conf, lr, err := eximcfg.Render(v, a.sInt("log_rotate_mb"), a.sInt("log_keep_files"))
	if err != nil {
		return err
	}
	return eximcfg.WriteDir(a.configDir, conf, lr, dkimKeys)
}

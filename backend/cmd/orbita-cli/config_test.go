package main

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestConfigMinima(t *testing.T) {
	c, errs := loadConfig(env(map[string]string{"ORBITA_HOSTNAME": "Relay.Exemplo.com.br"}))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if c.Mode != "relay" || c.Hostname != "relay.exemplo.com.br" || c.Selector != "dkim" || c.MaxMB != 25 || c.centralOn() {
		t.Errorf("padrões inesperados: %+v", c)
	}
}

func TestConfigErros(t *testing.T) {
	casos := map[string]map[string]string{
		"hostname":      {},
		"hostname_inv":  {"ORBITA_HOSTNAME": "relay"},
		"modo":          {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_MODE": "x"},
		"rede":          {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_RELAY_NETS": "10.0.0.0/99"},
		"removida":      {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_PUBLIC_IPS": "203.0.113.10"},
		"dominio":       {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_DOMAINS": "exemplo"},
		"smarthost":     {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_MODE": "smarthost"},
		"auth":          {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_MODE": "smarthost", "ORBITA_SMARTHOST_HOST": "s.x.com", "ORBITA_SMARTHOST_USER": "u"},
		"token":         {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_CENTRAL_URL": "https://c.exemplo.com"},
		"url":           {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_CENTRAL_URL": "ftp://x", "ORBITA_CENTRAL_TOKEN": "t"},
		"porta":         {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_SMARTHOST_PORT": "0"},
		"auto_removido": {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_RELAY_NETS_AUTO": "true"},
		"seletor":       {"ORBITA_HOSTNAME": "r.exemplo.com", "ORBITA_DKIM_SELECTOR": "a b"},
	}
	for nome, m := range casos {
		if _, errs := loadConfig(env(m)); len(errs) == 0 {
			t.Errorf("%s: deveria falhar", nome)
		}
	}
}

func TestConfigCompleta(t *testing.T) {
	c, errs := loadConfig(env(map[string]string{
		"ORBITA_HOSTNAME": "relay.exemplo.com.br", "ORBITA_DOMAINS": "Exemplo.com.br, filial.com.br,exemplo.com.br",
		"ORBITA_RELAY_NETS": "10.1.2.3/8 192.168.0.5", "ORBITA_HOST_IPS": "203.0.113.10",
		"ORBITA_CENTRAL_URL": "https://orbita.exemplo.com.br/sub/", "ORBITA_CENTRAL_TOKEN": "orb_x", "ORBITA_CENTRAL_INTERVAL": "5",
	}))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if strings.Join(c.Domains, ",") != "exemplo.com.br,filial.com.br" {
		t.Errorf("domínios: %v", c.Domains)
	}
	if strings.Join(c.effectiveNets(), ",") != "127.0.0.1,::1,10.0.0.0/8,192.168.0.5,203.0.113.10" {
		t.Errorf("redes: %v", c.effectiveNets())
	}
	if c.spfRecord() != "v=spf1 a:relay.exemplo.com.br ~all" {
		t.Errorf("spf: %s", c.spfRecord())
	}
	if c.CentralPort != 443 || c.CentralPath != "/sub" || c.CentralEvery.Seconds() != 5 {
		t.Errorf("central: %+v", c)
	}
	if c.dmarcRecord() != "v=DMARC1; p=none; adkim=r; aspf=r; pct=100" {
		t.Errorf("dmarc: %s", c.dmarcRecord())
	}
}

func TestSPFPadraoUsaHostname(t *testing.T) {
	c, _ := loadConfig(env(map[string]string{"ORBITA_HOSTNAME": "relay.exemplo.com.br"}))
	if c.spfRecord() != "v=spf1 a:relay.exemplo.com.br ~all" {
		t.Error(c.spfRecord())
	}
}

func TestViewSmarthostEscapaSenha(t *testing.T) {
	c, errs := loadConfig(env(map[string]string{"ORBITA_HOSTNAME": "relay.exemplo.com.br", "ORBITA_MODE": "smarthost",
		"ORBITA_SMARTHOST_HOST": "smtp.x.com", "ORBITA_SMARTHOST_USER": "u@x.com", "ORBITA_SMARTHOST_PASS": `a:b$c`}))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	v := c.view(nil)
	if !v.Smarthost || !v.HasAuth || v.Pass != `a::b\$c` || v.DKIMDomains != "" {
		t.Errorf("view: %+v", v)
	}
}

func TestEnforceDesligaSemDominios(t *testing.T) {
	c, _ := loadConfig(env(map[string]string{"ORBITA_HOSTNAME": "relay.exemplo.com.br"}))
	if c.Enforce || c.signing() {
		t.Error("sem domínios não deve restringir nem assinar")
	}
}

func TestRedesPadraoSoLoopback(t *testing.T) {
	c, errs := loadConfig(env(map[string]string{"ORBITA_HOSTNAME": "relay.exemplo.com.br"}))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if strings.Join(c.effectiveNets(), ",") != "127.0.0.1,::1" {
		t.Errorf("padrão deveria ser só loopback: %v", c.effectiveNets())
	}
}

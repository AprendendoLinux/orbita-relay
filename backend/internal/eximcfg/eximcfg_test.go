package eximcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cases() map[string]View {
	return map[string]View{
		"relay-basico": {Hostname: "relay.exemplo.com.br", RelayNets: "127.0.0.1 ; ::1 ; 10.0.0.0/8 ; 192.168.0.0/16", Gateways: "172.18.0.1 ; fd00::1", MaxMB: 25,
			SenderDomains: "exemplo.com.br ; filial.exemplo.com.br", DKIMDomains: "exemplo.com.br ; filial.exemplo.com.br", Selector: "dkim"},
		"relay-ipv4-simples": {Hostname: "", IPv4Only: true, RelayNets: "127.0.0.1", MaxMB: 10, Selector: "orbita"},
		"smarthost-starttls-auth": {Hostname: "relay.exemplo.com.br", RelayNets: "127.0.0.1 ; 10.0.0.0/8", Gateways: "172.18.0.1", MaxMB: 25, Smarthost: true,
			SmarthostHost: "smtp-relay.gmail.com", Port: 587, TLS: "starttls", HasAuth: true, User: EscClientSend("user@x.com"), Pass: EscClientSend(`p:a$s\s`), Selector: "dkim"},
		"smarthost-tls-semauth": {Hostname: "r.exemplo.com", RelayNets: "127.0.0.1", MaxMB: 25, Smarthost: true, SmarthostHost: "smtp.exemplo.com", Port: 465, TLS: "tls", Selector: "dkim"},
		"smarthost-none":        {Hostname: "r.exemplo.com", RelayNets: "127.0.0.1", MaxMB: 25, Smarthost: true, SmarthostHost: "10.0.0.5", Port: 25, TLS: "none", Selector: "dkim"},
	}
}

// Os arquivos golden foram gerados pela implementação anterior ao refactor: a saída precisa ser idêntica.
func TestRenderIgualAoOriginal(t *testing.T) {
	for name, v := range cases() {
		conf, _, err := Render(v, 50, 7)
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", name+".conf.golden"))
		if err != nil {
			t.Fatal(err)
		}
		if string(conf) != string(want) {
			t.Errorf("%s: exim.conf difere do golden", name)
		}
	}
	_, lr, _ := Render(View{MaxMB: 1}, 50, 7)
	want, _ := os.ReadFile(filepath.Join("testdata", "logrotate.golden"))
	if string(lr) != string(want) {
		t.Error("logrotate.conf difere do golden")
	}
}

func TestLogrotateLimites(t *testing.T) {
	_, lr, _ := Render(View{}, 0, 0)
	if !strings.Contains(string(lr), "maxsize 1M") || !strings.Contains(string(lr), "rotate 1") {
		t.Error("valores mínimos não aplicados")
	}
}

func TestEscClientSend(t *testing.T) {
	if got := EscClientSend(`a:b$c\d`); got != `a::b\$c\\d` {
		t.Errorf("escape incorreto: %q", got)
	}
}

func TestWriteDir(t *testing.T) {
	dir := t.TempDir()
	conf, lr, _ := Render(cases()["relay-basico"], 50, 7)
	keys := map[string]string{"exemplo.com.br": "CHAVE-A", "filial.exemplo.com.br": "CHAVE-B"}
	if err := WriteDir(dir, conf, lr, keys); err != nil {
		t.Fatal(err)
	}
	v1, _ := os.ReadFile(filepath.Join(dir, "version"))
	m, _ := os.ReadFile(filepath.Join(dir, "dkim", "map"))
	if string(m) != "exemplo.com.br: /etc/exim4/dkim/exemplo.com.br.key\nfilial.exemplo.com.br: /etc/exim4/dkim/filial.exemplo.com.br.key\n" {
		t.Errorf("mapa inesperado: %q", m)
	}
	if err := WriteDir(dir, conf, lr, keys); err != nil {
		t.Fatal(err)
	}
	v2, _ := os.ReadFile(filepath.Join(dir, "version"))
	if string(v1) != string(v2) {
		t.Error("a versão deve ser estável para o mesmo conteúdo")
	}
	keys["exemplo.com.br"] = "CHAVE-NOVA"
	_ = WriteDir(dir, conf, lr, keys)
	v3, _ := os.ReadFile(filepath.Join(dir, "version"))
	if string(v1) == string(v3) {
		t.Error("a versão deve mudar quando uma chave muda")
	}
	delete(keys, "filial.exemplo.com.br")
	_ = WriteDir(dir, conf, lr, keys)
	if _, err := os.Stat(filepath.Join(dir, "dkim", "filial.exemplo.com.br.key")); err == nil {
		t.Error("chave de domínio removido deveria ser apagada")
	}
	if fi, _ := os.Stat(filepath.Join(dir, "dkim", "exemplo.com.br.key")); fi == nil || fi.Mode().Perm() != 0o600 {
		t.Error("chave deve ter permissão 0600")
	}
}

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"orbitarelay/internal/dkimkey"
	"orbitarelay/internal/eximcfg"
)

const (
	eximConf   = "/etc/exim4/exim4.conf"
	eximDKIM   = "/etc/exim4/dkim"
	eximTLS    = "/etc/exim4/tls"
	logDir     = "/var/log/exim"
	renderDir  = "/run/orbita"
	logrotConf = "/etc/logrotate-exim.conf"
)

func (c Config) dkimDir() string { return filepath.Join(c.DataDir, "dkim") }

func (c Config) signing() bool { return c.Mode == "relay" && len(c.Domains) > 0 }

func sh(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c Config) ensureKeys() (map[string]string, error) {
	keys := map[string]string{}
	if !c.signing() {
		return keys, nil
	}
	if err := os.MkdirAll(c.dkimDir(), 0o700); err != nil {
		return nil, fmt.Errorf("criar %s: %w", c.dkimDir(), err)
	}
	for _, d := range c.Domains {
		kf := filepath.Join(c.dkimDir(), d+".key")
		b, err := os.ReadFile(kf)
		if err == nil {
			if _, e := dkimkey.PublicFromPEM(string(b)); e == nil {
				keys[d] = string(b)
				continue
			}
			return nil, fmt.Errorf("chave DKIM inválida em %s (remova o arquivo para gerar outra)", kf)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		priv, pub, err := dkimkey.Generate()
		if err != nil {
			return nil, err
		}
		if err := eximcfg.WriteAtomic(kf, []byte(priv), 0o600); err != nil {
			return nil, err
		}
		if err := eximcfg.WriteAtomic(filepath.Join(c.dkimDir(), d+".pub"), []byte(pub), 0o644); err != nil {
			return nil, err
		}
		logf("chave DKIM gerada para %s (seletor %s)", d, c.Selector)
		keys[d] = priv
	}
	return keys, nil
}

func (c Config) view(keys map[string]string) eximcfg.View {
	v := eximcfg.View{
		Hostname:  c.Hostname,
		IPv4Only:  c.IPv4Only,
		MaxMB:     c.MaxMB,
		Selector:  c.Selector,
		RelayNets: strings.Join(c.effectiveNets(), " ; "),
		Gateways:  strings.Join(gatewayIPs(), " ; "),
	}
	if c.Mode == "relay" {
		if c.Enforce {
			v.SenderDomains = strings.Join(c.Domains, " ; ")
		}
		v.DKIMDomains = strings.Join(c.Domains, " ; ")
		return v
	}
	v.Smarthost, v.SmarthostHost, v.Port, v.TLS = true, c.SHHost, c.SHPort, c.SHTLS
	if c.SHUser != "" && c.SHPass != "" {
		v.HasAuth = true
		v.User = eximcfg.EscClientSend(c.SHUser)
		v.Pass = eximcfg.EscClientSend(c.SHPass)
	}
	return v
}

func (c Config) prepare() error {
	for _, d := range []string{logDir, "/run/exim4", "/var/spool/exim4", eximTLS, eximDKIM, renderDir, c.DataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("criar %s: %w", d, err)
		}
	}
	if err := sh("chown", "-R", "Debian-exim:Debian-exim", logDir, "/run/exim4", "/var/spool/exim4"); err != nil {
		return err
	}
	for _, f := range []string{"main", "reject", "panic"} {
		p := filepath.Join(logDir, f+".log")
		if _, err := os.Stat(p); err != nil {
			if err := os.WriteFile(p, nil, 0o644); err != nil {
				return err
			}
		}
	}
	_ = sh("chown", "Debian-exim:Debian-exim", filepath.Join(logDir, "main.log"), filepath.Join(logDir, "reject.log"), filepath.Join(logDir, "panic.log"))
	return c.ensureTLS()
}

func (c Config) ensureTLS() error {
	persist := filepath.Join(c.DataDir, "tls")
	cert, key := filepath.Join(persist, "cert.pem"), filepath.Join(persist, "key.pem")
	if _, err := os.Stat(cert); err != nil {
		if err := os.MkdirAll(persist, 0o700); err != nil {
			return err
		}
		logf("gerando certificado TLS autoassinado")
		if err := sh("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "3650",
			"-subj", "/CN="+c.Hostname, "-keyout", key, "-out", cert); err != nil {
			return err
		}
	}
	for src, dst := range map[string]string{cert: filepath.Join(eximTLS, "cert.pem"), key: filepath.Join(eximTLS, "key.pem")} {
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o640); err != nil {
			return err
		}
	}
	return sh("chown", "Debian-exim:Debian-exim", filepath.Join(eximTLS, "cert.pem"), filepath.Join(eximTLS, "key.pem"))
}

func (c Config) apply() error {
	keys, err := c.ensureKeys()
	if err != nil {
		return err
	}
	conf, lr, err := eximcfg.Render(c.view(keys), c.LogRotateMB, c.LogKeep)
	if err != nil {
		return err
	}
	if err := eximcfg.WriteDir(renderDir, conf, lr, keys); err != nil {
		return err
	}
	tmp := filepath.Join(renderDir, "exim.conf")
	if out, err := exec.Command("exim4", "-C", tmp, "-bV").CombinedOutput(); err != nil {
		return fmt.Errorf("configuração do Exim inválida: %s", strings.TrimSpace(string(out)))
	}
	if err := os.RemoveAll(eximDKIM); err != nil {
		return err
	}
	if err := os.MkdirAll(eximDKIM, 0o750); err != nil {
		return err
	}
	ents, _ := os.ReadDir(filepath.Join(renderDir, "dkim"))
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(renderDir, "dkim", e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(eximDKIM, e.Name()), b, 0o640); err != nil {
			return err
		}
	}
	if err := sh("chown", "-R", "root:Debian-exim", eximDKIM); err != nil {
		return err
	}
	b, _ := os.ReadFile(tmp)
	if err := os.WriteFile(eximConf, b, 0o640); err != nil {
		return err
	}
	if err := sh("chown", "root:Debian-exim", eximConf); err != nil {
		return err
	}
	b, _ = os.ReadFile(filepath.Join(renderDir, "logrotate.conf"))
	return os.WriteFile(logrotConf, b, 0o644)
}

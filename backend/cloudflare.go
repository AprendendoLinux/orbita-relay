package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Faixas de IP do proxy da Cloudflare. Um nome atrás do proxy (nuvem laranja) resolve para
// esses IPs, e não para a máquina. Esses IPs nunca podem ser autorizados a enviar: são
// compartilhados por milhares de sites, e serviços hospedados na Cloudflare saem por eles.
//
// A lista é baixada de https://www.cloudflare.com/ips-v4 e ips-v6 ao iniciar e uma vez por dia.
// A lista abaixo é só a reserva usada se nunca houver download bem-sucedido.
var cloudflareFallback = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
}

const (
	cloudflareURLv4 = "https://www.cloudflare.com/ips-v4"
	cloudflareURLv6 = "https://www.cloudflare.com/ips-v6"
	cfRefreshEvery  = 24 * time.Hour
	cfRetryEvery    = time.Hour
	cfStaleAfter    = 7 * 24 * time.Hour
)

var cfState struct {
	sync.RWMutex
	nets      []netip.Prefix
	updated   time.Time // última atualização bem-sucedida; zero se nunca
	lastError string
}

func init() {
	cfState.nets = mustPrefixes(cloudflareFallback...)
}

func mustPrefixes(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(ss))
	for _, s := range ss {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

func isCloudflareIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	cfState.RLock()
	defer cfState.RUnlock()
	for _, p := range cfState.nets {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// parseCloudflareList lê uma lista de faixas (uma por linha). Qualquer linha inválida
// rejeita a lista inteira, e uma lista muito curta também, para não aceitar página de erro.
func parseCloudflareList(body string, minCount int) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		p, err := netip.ParsePrefix(l)
		if err != nil {
			return nil, fmt.Errorf("faixa inválida na lista da Cloudflare: %q", l)
		}
		out = append(out, p)
	}
	if len(out) < minCount {
		return nil, fmt.Errorf("lista da Cloudflare com só %d faixas", len(out))
	}
	return out, nil
}

func fetchCloudflareList(ctx context.Context, client *http.Client, url string, minCount int) ([]netip.Prefix, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "OrbitaRelay")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d em %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	return parseCloudflareList(string(body), minCount)
}

// refreshCloudflareRanges baixa as duas listas. Só troca a lista em uso se as duas derem certo.
func refreshCloudflareRanges(ctx context.Context) error {
	client := &http.Client{Timeout: 15 * time.Second}
	v4, err := fetchCloudflareList(ctx, client, cloudflareURLv4, 10)
	if err != nil {
		return err
	}
	v6, err := fetchCloudflareList(ctx, client, cloudflareURLv6, 5)
	if err != nil {
		return err
	}
	cfState.Lock()
	cfState.nets = append(v4, v6...)
	cfState.updated = time.Now()
	cfState.lastError = ""
	cfState.Unlock()
	return nil
}

// cloudflareStatus informa de onde vem a lista em uso, para a tela de verificação.
func cloudflareStatus() (source string, updated time.Time, stale bool) {
	cfState.RLock()
	defer cfState.RUnlock()
	if cfState.updated.IsZero() {
		return "reserva interna (não foi possível baixar a lista da Cloudflare)", time.Time{}, true
	}
	return "baixada da Cloudflare", cfState.updated, time.Since(cfState.updated) > cfStaleAfter
}

// runCloudflareRanges mantém a lista atualizada enquanto o app roda.
func (a *App) runCloudflareRanges(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		cfState.RLock()
		updated := cfState.updated
		cfState.RUnlock()
		if updated.IsZero() || time.Since(updated) >= cfRefreshEvery {
			if err := refreshCloudflareRanges(ctx); err != nil && ctx.Err() == nil {
				log.Printf("faixas da Cloudflare: %v (usando a lista em uso)", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"orbitarelay/internal/cloudflare"
)

// Mesmas regras do painel: o hostname do relay precisa apontar para IPs desta máquina,
// e IPs da Cloudflare nunca são autorizados a enviar.

// refreshCloudflare baixa a lista atual de faixas da Cloudflare (com reserva interna se falhar).
func refreshCloudflare(ctx context.Context) {
	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := cloudflare.Refresh(rctx); err != nil {
		logf("faixas da Cloudflare: %v (usando a lista de reserva)", err)
	}
}

// cloudflareErrors recusa ORBITA_RELAY_NETS e ORBITA_HOST_IPS que caiam em faixas da Cloudflare.
func (c Config) cloudflareErrors() []string {
	var errs []string
	check := func(name string, list []string) {
		for _, n := range list {
			ip := n
			if p, err := netip.ParsePrefix(n); err == nil {
				ip = p.Addr().String()
			}
			if a, err := netip.ParseAddr(ip); err == nil && cloudflare.IsCloudflare(a) {
				errs = append(errs, fmt.Sprintf("%s: %s é da Cloudflare e não pode ser autorizado (são IPs compartilhados)", name, n))
			}
		}
	}
	check("ORBITA_RELAY_NETS", c.RelayNets)
	check("ORBITA_HOST_IPS", c.HostIPs)
	return errs
}

// hostnameWarnings verifica o DNS do hostname do relay. São avisos: o relay sobe mesmo assim.
func (c Config) hostnameWarnings(ctx context.Context) []string {
	rctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	addrs, err := netipLookup(rctx, c.Hostname)
	var out []string
	if err != nil || len(addrs) == 0 {
		return append(out, fmt.Sprintf("%s não tem registro A/AAAA. O SPF (a:%s) não autoriza este servidor até o DNS apontar para ele.", c.Hostname, c.Hostname))
	}
	inDNS := map[string]bool{}
	var cf []string
	for _, a := range addrs {
		inDNS[a.String()] = true
		if cloudflare.IsCloudflare(a) {
			cf = append(cf, a.String())
		}
	}
	if len(cf) > 0 {
		out = append(out, fmt.Sprintf("%s passa pelo proxy da Cloudflare (%s). Use um registro DNS only apontando para o IP do servidor.", c.Hostname, strings.Join(cf, ", ")))
	}
	for _, ip := range c.HostIPs {
		p, err := netip.ParsePrefix(ip)
		addr := ip
		if err == nil {
			addr = p.Addr().String()
		}
		if a, err := netip.ParseAddr(addr); err == nil && !inDNS[a.Unmap().String()] {
			out = append(out, fmt.Sprintf("%s em ORBITA_HOST_IPS não aparece no DNS de %s. Confirme que é um IP desta máquina.", ip, c.Hostname))
		}
	}
	return out
}

func netipLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.Unmap())
	}
	return out, nil
}

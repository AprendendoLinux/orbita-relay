package main

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"os"
	"sort"
	"strings"
	"time"
)

type detNet struct {
	CIDR   string `json:"cidr"`
	Label  string `json:"label"`
	Source string `json:"source"`
}

func addrOK(ip netip.Addr) bool {
	return ip.IsValid() && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
}

func listFromEnv(k string) []string {
	return strings.FieldsFunc(os.Getenv(k), func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' })
}

func (a *App) knownHostIPs() []string {
	var out []string
	for _, l := range strings.Split(a.sStr("known_host_ips"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func (a *App) detectedNets() []detNet {
	var out []detNet
	seen := map[string]bool{}
	add := func(c, label, src string) {
		if !seen[c] {
			seen[c] = true
			out = append(out, detNet{c, label, src})
		}
	}
	add("127.0.0.1", "Loopback IPv4", "loopback")
	add("::1", "Loopback IPv6", "loopback")
	if ifs, err := net.Interfaces(); err == nil {
		for _, ifc := range ifs {
			if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := ifc.Addrs()
			for _, ad := range addrs {
				ipn, ok := ad.(*net.IPNet)
				if !ok {
					continue
				}
				ip, ok := netip.AddrFromSlice(ipn.IP)
				if !ok {
					continue
				}
				ip = ip.Unmap()
				if !addrOK(ip) {
					continue
				}
				ones, _ := ipn.Mask.Size()
				if ip.Is4() && ones > 32 {
					ones = 32
				}
				pfx := netip.PrefixFrom(ip, ones).Masked()
				add(pfx.String(), "Rede Docker do contêiner (interface "+ifc.Name+")", "interface")
			}
		}
	}
	for _, s := range listFromEnv("ORBITA_HOST_IPS") {
		if p, err := netip.ParsePrefix(s); err == nil {
			add(p.Masked().String(), "Informado em ORBITA_HOST_IPS", "env")
		} else if ip, err := netip.ParseAddr(s); err == nil && addrOK(ip.Unmap()) {
			add(ip.Unmap().String(), "IP da máquina (ORBITA_HOST_IPS)", "env")
		}
	}
	for _, s := range a.knownHostIPs() {
		if ip, err := netip.ParseAddr(s); err == nil {
			add(ip.Unmap().String(), "IP da máquina (usado para acessar o painel)", "painel")
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return false })
	return out
}

func (a *App) effectiveRelayNets() []string {
	var nets []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			nets = append(nets, s)
		}
	}
	for _, l := range strings.Split(a.sStr("relay_nets"), "\n") {
		add(strings.TrimSpace(l))
	}
	if a.sBool("relay_nets_auto") {
		for _, d := range a.detectedNets() {
			add(d.CIDR)
		}
	}
	if len(nets) == 0 {
		nets = []string{"127.0.0.1", "::1"}
	}
	return nets
}

func hostIPFromRequest(r *http.Request) string {
	h := r.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.Trim(h, "[]")
	ip, err := netip.ParseAddr(h)
	if err != nil {
		return ""
	}
	ip = ip.Unmap()
	if !addrOK(ip) || ip.IsLoopback() {
		return ""
	}
	return ip.String()
}

func (a *App) handleDetectNets(w http.ResponseWriter, r *http.Request) {
	if a.sBool("relay_nets_auto") {
		if ip := hostIPFromRequest(r); ip != "" {
			known := a.knownHostIPs()
			have := false
			for _, k := range known {
				if k == ip {
					have = true
				}
			}
			if !have {
				known = append(known, ip)
				if len(known) > 8 {
					known = known[len(known)-8:]
				}
				if err := a.setRaw(r.Context(), "known_host_ips", strings.Join(known, "\n")); err == nil {
					_ = a.renderMTA(r.Context())
				}
			}
		}
	}
	items := a.detectedNets()
	have := map[string]bool{}
	for _, d := range items {
		have[d.CIDR] = true
	}
	writeJSON(w, 200, map[string]any{"auto": a.sBool("relay_nets_auto"), "items": items, "suggestions": a.suggestedNets(r.Context(), r, have), "effective": a.effectiveRelayNets()})
}

type dockerNet struct {
	Name, Driver string
	Subnets      []string
	Gateways     []string
}

func dockerNetworks(ctx context.Context) []dockerNet {
	base := strings.TrimRight(os.Getenv("ORBITA_DOCKER_API"), "/")
	if base == "" {
		return nil
	}
	var raw []struct {
		Name   string
		Driver string
		IPAM   struct {
			Config []struct{ Subnet, Gateway string }
		}
	}
	if dockerGet(ctx, base, "/networks", &raw) != nil {
		return nil
	}
	var out []dockerNet
	for _, n := range raw {
		if n.Driver == "host" || n.Driver == "null" {
			continue
		}
		dn := dockerNet{Name: n.Name, Driver: n.Driver}
		for _, c := range n.IPAM.Config {
			if p, err := netip.ParsePrefix(c.Subnet); err == nil {
				dn.Subnets = append(dn.Subnets, p.Masked().String())
			}
			if g, err := netip.ParseAddr(c.Gateway); err == nil {
				dn.Gateways = append(dn.Gateways, g.String())
			}
		}
		if len(dn.Subnets) > 0 {
			out = append(out, dn)
		}
	}
	return out
}

func (a *App) suggestedNets(ctx context.Context, r *http.Request, have map[string]bool) []detNet {
	out := []detNet{}
	add := func(c, label, src string) {
		if c != "" && !have[c] {
			have[c] = true
			out = append(out, detNet{c, label, src})
		}
	}
	for _, n := range dockerNetworks(ctx) {
		for _, s := range n.Subnets {
			add(s, "Rede Docker «"+n.Name+"» (outra rede do host)", "docker")
		}
		for _, g := range n.Gateways {
			add(g, "IP do host na rede Docker «"+n.Name+"»", "host")
		}
	}
	name := r.Host
	if hh, _, err := net.SplitHostPort(name); err == nil {
		name = hh
	}
	name = strings.Trim(name, "[]")
	if _, err := netip.ParseAddr(name); err != nil && name != "" && !strings.EqualFold(name, "localhost") {
		lc, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if ips, err := net.DefaultResolver.LookupHost(lc, name); err == nil {
			for i, s := range ips {
				if i >= 6 {
					break
				}
				if ip, err := netip.ParseAddr(s); err == nil && addrOK(ip.Unmap()) && !ip.IsLoopback() {
					add(ip.Unmap().String(), "IP de "+name+" (DNS; confira se é desta máquina)", "dns")
				}
			}
		}
	}
	return out
}

func (a *App) mergedRelayNetsText(r *http.Request) string {
	var lines []string
	seen := map[string]bool{}
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" && !seen[s] {
			seen[s] = true
			lines = append(lines, s)
		}
	}
	for _, l := range strings.Split(a.sStr("relay_nets"), "\n") {
		add(l)
	}
	have := map[string]bool{}
	for _, d := range a.detectedNets() {
		have[d.CIDR] = true
		add(d.CIDR)
	}
	for _, d := range a.suggestedNets(r.Context(), r, have) {
		add(d.CIDR)
	}
	return strings.Join(lines, "\n")
}

func hostnameFromRequest(r *http.Request) string {
	h := r.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.ToLower(strings.Trim(h, "[]"))
	if h == "" || !strings.Contains(h, ".") || strings.EqualFold(h, "localhost") {
		return ""
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return ""
	}
	return h
}

// Package netdet detecta gateways e redes do contêiner.
package netdet

import (
	"bufio"
	"encoding/hex"
	"net"
	"net/netip"
	"os"
	"strings"
)

// ReadDefaultGateways lista os gateways padrão (IPv4 e IPv6) lidos de /proc.
func ReadDefaultGateways() []string {
	var out []string
	if f, err := os.Open("/proc/net/route"); err == nil {
		sc := bufio.NewScanner(f)
		sc.Scan()
		for sc.Scan() {
			c := strings.Fields(sc.Text())
			if len(c) < 3 || c[1] != "00000000" || c[2] == "00000000" {
				continue
			}
			if b, err := hex.DecodeString(c[2]); err == nil && len(b) == 4 {
				out = append(out, net.IPv4(b[3], b[2], b[1], b[0]).String())
			}
		}
		f.Close()
	}
	if f, err := os.Open("/proc/net/ipv6_route"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			c := strings.Fields(sc.Text())
			if len(c) < 6 || c[0] != strings.Repeat("0", 32) || c[1] != "00" || c[4] == strings.Repeat("0", 32) {
				continue
			}
			if b, err := hex.DecodeString(c[4]); err == nil && len(b) == 16 {
				out = append(out, net.IP(b).String())
			}
		}
		f.Close()
	}
	return out
}

// DockerGateways devolve os gateways que o Exim deve recusar (origem mascarada pelo docker-proxy).
func DockerGateways() []string {
	if v := os.Getenv("ORBITA_DOCKER_GATEWAYS"); v != "" {
		var out []string
		for _, p := range strings.Split(v, ",") {
			if ip := net.ParseIP(strings.TrimSpace(p)); ip != nil {
				out = append(out, ip.String())
			}
		}
		return out
	}
	seen := map[string]bool{}
	var out []string
	for _, g := range ReadDefaultGateways() {
		if !seen[g] && !strings.HasPrefix(g, "fe80:") {
			seen[g] = true
			out = append(out, g)
		}
	}
	return out
}

// InterfaceNets devolve as redes das interfaces ativas (exceto loopback), mascaradas.
func InterfaceNets() []string {
	var out []string
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
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
			if !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			ones, _ := ipn.Mask.Size()
			if ip.Is4() && ones > 32 {
				ones = 32
			}
			out = append(out, netip.PrefixFrom(ip, ones).Masked().String())
		}
	}
	return out
}

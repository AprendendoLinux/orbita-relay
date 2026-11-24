package main

import (
	"context"
	"errors"
	"log"
	"net/netip"
	"os"
	"strings"
	"time"
)

// Todas as redes Docker (sub-redes) lidas pelo proxy de API (ORBITA_DOCKER_API) entram
// automaticamente na lista "Redes autorizadas a enviar". A lista de redes Docker
// já sincronizadas fica em dockerNetsKey: só o que surge ou some no Docker é
// alterado. Se você remover uma rede da lista, ela não volta enquanto continuar
// existindo no Docker.
const dockerNetsKey = "relay_nets_docker"

var errNoDockerAPI = errors.New("ORBITA_DOCKER_API não configurado")

type dockerNet struct {
	Name, Driver string
	Subnets      []string
}

// dockerNetworks devolve as redes Docker com sub-rede IPv4/IPv6 (sem host/null).
// Os gateways não entram: são IPs de origem mascarada, e o Exim já os recusa.
func dockerNetworks(ctx context.Context) ([]dockerNet, error) {
	base := strings.TrimRight(os.Getenv("ORBITA_DOCKER_API"), "/")
	if base == "" {
		return nil, errNoDockerAPI
	}
	var raw []struct {
		Name   string
		Driver string
		IPAM   struct {
			Config []struct{ Subnet string }
		}
	}
	if err := dockerGet(ctx, base, "/networks", &raw); err != nil {
		return nil, err
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
		}
		if len(dn.Subnets) > 0 {
			out = append(out, dn)
		}
	}
	return out, nil
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// effectiveRelayNets é a lista usada pelo Exim: a lista salva, ou loopback se estiver vazia.
func (a *App) effectiveRelayNets() []string {
	nets := splitLines(a.sStr("relay_nets"))
	if len(nets) == 0 {
		nets = []string{"127.0.0.1", "::1"}
	}
	return nets
}

// syncDockerNetworks acrescenta à lista as redes Docker novas e retira as que sumiram.
// Devolve true se a lista mudou (e o Exim precisa ser gerado de novo).
func (a *App) syncDockerNetworks(ctx context.Context) (bool, error) {
	nets, err := dockerNetworks(ctx)
	if err != nil {
		return false, err
	}
	var desired []string
	inDesired := map[string]bool{}
	for _, n := range nets {
		for _, s := range n.Subnets {
			if !inDesired[s] {
				inDesired[s] = true
				desired = append(desired, s)
			}
		}
	}
	prev := splitLines(a.sStr(dockerNetsKey))
	out, changed := mergeDockerNets(splitLines(a.sStr("relay_nets")), prev, desired)
	if strings.Join(prev, "\n") != strings.Join(desired, "\n") {
		if err := a.setRaw(ctx, dockerNetsKey, strings.Join(desired, "\n")); err != nil {
			return false, err
		}
	}
	if changed {
		if err := a.setRaw(ctx, "relay_nets", strings.Join(out, "\n")); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// mergeDockerNets aplica a diferença entre as redes Docker anteriores (prev) e as atuais
// (desired) sobre a lista (cur): acrescenta as novas e retira as que sumiram. Uma rede
// que já estava na lista e não é nova nem sumiu é mantida como está (inclusive se você a removeu).
func mergeDockerNets(cur, prev, desired []string) ([]string, bool) {
	inPrev, inDesired, inCur := toSet(prev), toSet(desired), toSet(cur)
	changed := false
	var out []string
	for _, c := range cur {
		if inPrev[c] && !inDesired[c] { // rede Docker que sumiu
			changed = true
			continue
		}
		out = append(out, c)
	}
	for _, d := range desired {
		if !inPrev[d] && !inCur[d] { // rede Docker nova
			out = append(out, d)
			changed = true
		}
	}
	if len(out) == 0 {
		out = []string{"127.0.0.1", "::1"}
	}
	return out, changed
}

func toSet(l []string) map[string]bool {
	m := make(map[string]bool, len(l))
	for _, v := range l {
		m[v] = true
	}
	return m
}

// runDockerNets mantém a lista sincronizada com o Docker enquanto o app roda.
func (a *App) runDockerNets(ctx context.Context) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	lastErr := ""
	for {
		if a.ready() && os.Getenv("ORBITA_DOCKER_API") != "" {
			changed, err := a.syncDockerNetworks(ctx)
			if err != nil {
				if msg := err.Error(); msg != lastErr {
					log.Printf("redes Docker: %v", err)
					lastErr = msg
				}
			} else {
				lastErr = ""
			}
			if changed {
				if err := a.renderMTA(ctx); err != nil {
					log.Printf("redes Docker: %v", err)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

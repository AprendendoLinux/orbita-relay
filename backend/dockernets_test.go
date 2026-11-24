package main

import (
	"reflect"
	"testing"
)

func TestMergeDockerNets(t *testing.T) {
	cases := []struct {
		name               string
		cur, prev, desired []string
		wantOut            []string
		wantChanged        bool
	}{
		{"primeira sincronização adiciona tudo",
			[]string{"127.0.0.1", "::1"}, nil, []string{"172.18.0.0/16", "fd00::/64"},
			[]string{"127.0.0.1", "::1", "172.18.0.0/16", "fd00::/64"}, true},
		{"sem mudança no Docker não altera nada",
			[]string{"127.0.0.1", "::1", "172.18.0.0/16"}, []string{"172.18.0.0/16"}, []string{"172.18.0.0/16"},
			[]string{"127.0.0.1", "::1", "172.18.0.0/16"}, false},
		{"rede Docker que sumiu sai da lista",
			[]string{"127.0.0.1", "172.18.0.0/16", "172.19.0.0/16"}, []string{"172.18.0.0/16", "172.19.0.0/16"}, []string{"172.19.0.0/16"},
			[]string{"127.0.0.1", "172.19.0.0/16"}, true},
		{"rede removida à mão não volta enquanto existir",
			[]string{"127.0.0.1"}, []string{"172.18.0.0/16"}, []string{"172.18.0.0/16"},
			[]string{"127.0.0.1"}, false},
		{"rede manual que coincide com uma Docker nova não duplica",
			[]string{"127.0.0.1", "172.18.0.0/16"}, nil, []string{"172.18.0.0/16"},
			[]string{"127.0.0.1", "172.18.0.0/16"}, false},
		{"lista vazia volta ao loopback",
			[]string{"172.18.0.0/16"}, []string{"172.18.0.0/16"}, nil,
			[]string{"127.0.0.1", "::1"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed := mergeDockerNets(c.cur, c.prev, c.desired)
			if !reflect.DeepEqual(out, c.wantOut) || changed != c.wantChanged {
				t.Fatalf("got (%v, %v), want (%v, %v)", out, changed, c.wantOut, c.wantChanged)
			}
		})
	}
}

func TestDockerNetsAllowedDefault(t *testing.T) {
	t.Setenv("ORBITA_DOCKER_NETS", "")
	got := dockerNetsAllowed()
	if !got["echelon"] || len(got) != 1 {
		t.Fatalf("padrão deveria ser só echelon, got %v", got)
	}
	t.Setenv("ORBITA_DOCKER_NETS", " echelon , outra ")
	got = dockerNetsAllowed()
	if !got["echelon"] || !got["outra"] || got["bridge"] {
		t.Fatalf("lista configurada incorreta: %v", got)
	}
}

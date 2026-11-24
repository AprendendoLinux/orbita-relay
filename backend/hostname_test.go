package main

import (
	"reflect"
	"testing"
)

func TestApplyTrackedNets(t *testing.T) {
	cases := []struct {
		name               string
		cur, prev, desired []string
		want               []string
	}{
		{"autoriza IPs novos do hostname",
			[]string{"127.0.0.1", "::1"}, nil, []string{"187.110.161.52/32"},
			[]string{"127.0.0.1", "::1", "187.110.161.52/32"}},
		{"troca IP antigo do hostname pelo novo",
			[]string{"127.0.0.1", "187.110.161.52/32"}, []string{"187.110.161.52/32"}, []string{"203.0.113.7/32"},
			[]string{"127.0.0.1", "203.0.113.7/32"}},
		{"entrada digitada à mão não sai",
			[]string{"127.0.0.1", "10.0.0.5/32", "187.110.161.52/32"}, []string{"187.110.161.52/32"}, nil,
			[]string{"127.0.0.1", "10.0.0.5/32"}},
		{"sem duplicar quando já está na lista",
			[]string{"127.0.0.1", "187.110.161.52/32"}, nil, []string{"187.110.161.52/32"},
			[]string{"127.0.0.1", "187.110.161.52/32"}},
		{"sem nada sobra o loopback",
			[]string{"187.110.161.52/32"}, []string{"187.110.161.52/32"}, nil,
			[]string{"127.0.0.1", "::1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := applyTrackedNets(c.cur, c.prev, c.desired); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

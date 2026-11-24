package cloudflare

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

func TestIsCloudflare(t *testing.T) {
	cases := map[string]bool{
		"172.67.152.11":             true,
		"104.21.33.247":             true,
		"2606:4700:3030::6815:21f7": true,
		"187.110.161.52":            false,
		"2804:1740:11:1700::5":      false,
		"::ffff:172.67.152.11":      true,
		"10.0.0.5":                  false,
	}
	for s, want := range cases {
		if got := IsCloudflare(netip.MustParseAddr(s)); got != want {
			t.Errorf("%s: got %v, want %v", s, got, want)
		}
	}
}

func TestParseList(t *testing.T) {
	var v4 []string
	for i := 0; i < 12; i++ {
		v4 = append(v4, fmt.Sprintf("10.%d.0.0/16", i))
	}
	if got, err := ParseList(strings.Join(v4, "\n")+"\n", 10); err != nil || len(got) != 12 {
		t.Fatalf("lista válida: got %d, err %v", len(got), err)
	}
	if _, err := ParseList("<html>erro</html>", 1); err == nil {
		t.Error("HTML deveria ser rejeitado")
	}
	if _, err := ParseList("10.0.0.0/8\n", 10); err == nil {
		t.Error("lista curta deveria ser rejeitada")
	}
}

package main

import "testing"

func TestCloudflareRecusadoNasRedes(t *testing.T) {
	c, errs := loadConfig(env(map[string]string{
		"ORBITA_HOSTNAME":   "relay.exemplo.com.br",
		"ORBITA_RELAY_NETS": "172.67.152.11",
		"ORBITA_HOST_IPS":   "187.110.161.52",
	}))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	got := c.cloudflareErrors()
	if len(got) != 1 {
		t.Fatalf("esperava 1 erro (IP da Cloudflare em ORBITA_RELAY_NETS), got %v", got)
	}
}

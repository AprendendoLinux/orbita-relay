package main

import (
	"strings"
	"testing"
	"time"
)

func TestCountMaskedLinesJanelaDe7Dias(t *testing.T) {
	now := time.Date(2026, 12, 2, 12, 0, 0, 0, time.UTC)
	since := now.Add(-maskedWindow)
	log := strings.Join([]string{
		"2026-11-23 22:43:07 +0000 H=(echelon) [192.168.171.1] rejected MAIL <x@y>: Connection through Docker gateway refused (source IP masked)",
		"2026-11-29 08:00:00 +0000 H=(echelon) [192.168.171.1] rejected MAIL <x@y>: Connection through Docker gateway refused (source IP masked)",
		"2026-11-29 08:00:01 +0000 H=(outro) [187.110.161.52] rejected MAIL <x@y>: Relay not permitted",
		"2026-11-30 11:00:00 +0000 H=(echelon) [192.168.171.1] rejected MAIL <x@y>: Connection through Docker gateway refused (source IP masked)",
	}, "\n")
	n, last := countMaskedLines(strings.NewReader(log), since)
	if n != 2 {
		t.Fatalf("esperava 2 recusas dentro da janela, got %d", n)
	}
	if last != "2026-11-30 11:00:00" {
		t.Errorf("última recusa: %q", last)
	}
}

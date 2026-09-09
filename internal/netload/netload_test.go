package netload

import (
	"strings"
	"testing"
	"time"
)

func TestSkipIface(t *testing.T) {
	skip := []string{"lo", "lo0", "docker0", "br-abc", "veth123", "tun0", "tap0", "awg0", "amnezia", "wg0"}
	for _, n := range skip {
		if !skipIface(n) {
			t.Fatalf("expected skip %s", n)
		}
	}
	keep := []string{"eth0", "ens3", "enp0s3", "net0"}
	for _, n := range keep {
		if skipIface(n) {
			t.Fatalf("should keep %s", n)
		}
	}
}

func TestHumanRate(t *testing.T) {
	got := humanRate(27.5 * 1024 * 1024)
	if !strings.Contains(got, "MiB/s") || !strings.Contains(got, "Mbit/s") {
		t.Fatalf("got %q", got)
	}
}

func TestRateLocked(t *testing.T) {
	s := &Sampler{iface: "net0"}
	t0 := time.Now()
	s.hist = []Sample{
		{T: t0, Rx: 0, Tx: 0},
		{T: t0.Add(10 * time.Second), Rx: 10_000_000, Tx: 5_000_000},
	}
	rx, tx := s.rateLocked(10 * time.Second)
	if rx < 900_000 || rx > 1_100_000 {
		t.Fatalf("rx %v", rx)
	}
	if tx < 400_000 || tx > 600_000 {
		t.Fatalf("tx %v", tx)
	}
}

func TestFormatContainsWindows(t *testing.T) {
	r := Rates{Iface: "net0", NowRx: 1000, NowTx: 2000}
	got := r.Format()
	for _, needle := range []string{"Сеть", "net0", "Сейчас", "1 мин", "15 мин", "1 час", "↓", "↑"} {
		if !strings.Contains(got, needle) {
			t.Fatalf("missing %q in %s", needle, got)
		}
	}
}

func TestFormatCompact(t *testing.T) {
	r := Rates{Iface: "eth0", NowRx: 1000, NowTx: 2000}
	got := r.FormatCompact()
	if !strings.Contains(got, "eth0") || !strings.Contains(got, "↓") || !strings.Contains(got, "↑") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "15 мин") {
		t.Fatalf("compact should not list windows: %s", got)
	}
}

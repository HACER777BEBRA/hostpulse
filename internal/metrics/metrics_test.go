package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	d := 35*time.Hour + 11*time.Minute
	got := FormatDuration(d)
	if got != "1d 11h 11m" {
		t.Fatalf("got %q", got)
	}
	got = FormatDuration(3*time.Hour + 2*time.Minute + 1*time.Second)
	if got != "3h 2m 1s" {
		t.Fatalf("got %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	if got := HumanBytes(0); got != "0 B" {
		t.Fatalf("got %q", got)
	}
	if got := HumanBytes(5307 * 100 * 1024 / 10); !strings.Contains(got, "MiB") {
		t.Fatalf("got %q", got)
	}
}

func TestFormatStatusMatchesLayout(t *testing.T) {
	s := Snapshot{
		Hostname:    "amneziavpn.aeza.network",
		Uptime:      35*time.Hour + 11*time.Minute,
		BootTime:    time.Date(2026, 8, 25, 22, 57, 0, 0, time.UTC),
		CPUPercent:  92.8,
		Load1:       1.40,
		Load5:       0.55,
		Load15:      0.25,
		MemPercent:  13.6,
		MemUsed:     556 * 1024 * 1024,
		MemTotal:    4 * 1024 * 1024 * 1024,
		SwapPercent: 0,
		SwapUsed:    0,
		SwapTotal:   2 * 1024 * 1024 * 1024,
		DiskPercent: 69.4,
		DiskFree:    3 * 1024 * 1024 * 1024,
		DiskTotal:   10 * 1024 * 1024 * 1024,
	}
	got := s.FormatStatus()
	need := []string{
		"🖥 <b>amneziavpn.aeza.network</b>",
		"⏱ Uptime: <code>1d 11h 11m</code>",
		"📅 Boot: <code>2026-08-25 22:57 UTC</code>",
		"⚙️ CPU: <b>92.8%</b>",
		"📈 Load: <code>1.40 0.55 0.25</code>",
		"🧠 RAM:",
		"💾 Swap:",
		"💽 Disk:",
		"free",
	}
	for _, n := range need {
		if !strings.Contains(got, n) {
			t.Fatalf("missing %q in\n%s", n, got)
		}
	}
}

func TestFormatStatusCompact(t *testing.T) {
	s := Snapshot{
		Hostname:    "box",
		Uptime:      time.Hour,
		CPUPercent:  10.5,
		MemPercent:  20.1,
		DiskPercent: 30,
		Load1:       0.25,
	}
	got := s.FormatStatusCompact()
	for _, n := range []string{"<b>box</b>", "CPU <b>10.5%</b>", "RAM <b>20.1%</b>", "Disk <b>30.0%</b>", "Load <code>0.25</code>"} {
		if !strings.Contains(got, n) {
			t.Fatalf("missing %q in %s", n, got)
		}
	}
}

func TestFormatProcesses(t *testing.T) {
	s := Snapshot{
		Uptime:   time.Hour,
		BootTime: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC),
		TopCPU:   []Proc{{PID: 1, Name: "systemd", CPU: 12.3, RSS: 10 << 20}},
		TopRAM:   []Proc{{PID: 2, Name: "dockerd", CPU: 1.5, RSS: 200 << 20}},
	}
	got := s.FormatProcesses()
	if !strings.Contains(got, "Топ по CPU") || !strings.Contains(got, "pid=1") {
		t.Fatalf("cpu block: %s", got)
	}
	if !strings.Contains(got, "Топ по RAM") || !strings.Contains(got, "/ CPU 1.5%") {
		t.Fatalf("ram block: %s", got)
	}
}

func TestEscape(t *testing.T) {
	if got := Escape("<b>&"); got != "&lt;b&gt;&amp;" {
		t.Fatalf("got %q", got)
	}
}

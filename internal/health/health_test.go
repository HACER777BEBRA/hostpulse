package health

import (
	"strings"
	"testing"
)

func TestFormatReport(t *testing.T) {
	r := Report{
		DockerOK: true,
		Services: []ServiceStatus{
			{Name: "docker", Active: true, State: "active"},
			{Name: "ssh", Active: true, State: "active"},
			{Name: "fail2ban", Active: true, State: "active"},
			{Name: "amnezia-ensure", Active: true, State: "active"},
		},
		Containers: []ContainerStatus{
			{Name: "amnezia-awg2", Running: true, State: "running"},
		},
	}
	got := r.Format()
	for _, n := range []string{
		"🛡 <b>Сервисы</b>",
		"✅ <code>docker</code> — active",
		"✅ <code>ssh</code> — active",
		"🐳 <b>Контейнеры</b>",
		"✅ <code>amnezia-awg2</code> — running",
	} {
		if !strings.Contains(got, n) {
			t.Fatalf("missing %q in\n%s", n, got)
		}
	}
}

func TestFormatDockerUnavailable(t *testing.T) {
	r := Report{DockerOK: false, Containers: []ContainerStatus{{Name: "x"}}}
	got := r.Format()
	if !strings.Contains(got, "❌ Docker недоступен") {
		t.Fatalf("got %s", got)
	}
}

func TestFormatCompact(t *testing.T) {
	ok := Report{
		Services:   []ServiceStatus{{Name: "ssh", Active: true, State: "active"}},
		Containers: []ContainerStatus{{Name: "web", Running: true, State: "running"}},
	}
	if got := ok.FormatCompact(); got != "🛡 Всё ок (2)" {
		t.Fatalf("ok: %q", got)
	}
	bad := Report{
		Services:   []ServiceStatus{{Name: "ssh", Active: false, State: "failed"}},
		Containers: []ContainerStatus{{Name: "web", Running: false, State: "exited"}},
	}
	got := bad.FormatCompact()
	if !strings.Contains(got, "Проблемы") || !strings.Contains(got, "ssh") || !strings.Contains(got, "web") {
		t.Fatalf("bad: %s", got)
	}
}

func TestEscapePreTruncates(t *testing.T) {
	s := strings.Repeat("a", 4000)
	got := EscapePre(s)
	if !strings.HasSuffix(got, "…") {
		t.Fatal("expected truncation")
	}
}

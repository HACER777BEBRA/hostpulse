package health

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"hostpulse/internal/metrics"
)

type ServiceStatus struct {
	Name   string
	Active bool
	State  string
}

type ContainerStatus struct {
	Name    string
	Running bool
	State   string
}

type Report struct {
	Services   []ServiceStatus
	Containers []ContainerStatus
	DockerOK   bool
}

func Check(services, containers []string) Report {
	r := Report{}
	if len(containers) > 0 {
		r.DockerOK = dockerAvailable()
	}
	for _, name := range services {
		r.Services = append(r.Services, checkService(name))
	}
	for _, name := range containers {
		r.Containers = append(r.Containers, checkContainer(name, r.DockerOK))
	}
	return r
}

func dockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

func checkService(name string) ServiceStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "is-active", name).Output()
	state := strings.TrimSpace(string(out))
	if state == "" {
		if err != nil {
			state = "not found"
		} else {
			state = "unknown"
		}
	}
	return ServiceStatus{Name: name, Active: state == "active", State: state}
}

func checkContainer(name string, dockerOK bool) ContainerStatus {
	if !dockerOK {
		return ContainerStatus{Name: name, State: "docker unavailable"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", name).Output()
	state := strings.TrimSpace(string(out))
	if err != nil || state == "" {
		state = "not found"
	}
	return ContainerStatus{Name: name, Running: state == "running", State: state}
}

func (r Report) Format() string {
	var b strings.Builder
	b.WriteString(r.FormatServicesOnly())
	b.WriteString("\n\n🐳 <b>Контейнеры</b>\n")
	if len(r.Containers) == 0 {
		b.WriteString("Контейнеров нет")
		return b.String()
	}
	if !r.DockerOK {
		b.WriteString("❌ Docker недоступен")
		return b.String()
	}
	for i, c := range r.Containers {
		mark := "❌"
		if c.Running {
			mark = "✅"
		}
		fmt.Fprintf(&b, "%s <code>%s</code> — %s", mark, metrics.Escape(c.Name), metrics.Escape(c.State))
		if i+1 < len(r.Containers) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (r Report) FormatCompact() string {
	var parts []string
	down := 0
	for _, s := range r.Services {
		if !s.Active {
			down++
			parts = append(parts, fmt.Sprintf("❌ <code>%s</code>", metrics.Escape(s.Name)))
		}
	}
	for _, c := range r.Containers {
		if !c.Running {
			down++
			parts = append(parts, fmt.Sprintf("❌ <code>%s</code>", metrics.Escape(c.Name)))
		}
	}
	if down == 0 {
		n := len(r.Services) + len(r.Containers)
		if n == 0 {
			return "🛡 Сервисы: нет списка"
		}
		return fmt.Sprintf("🛡 Всё ок (%d)", n)
	}
	return "🛡 Проблемы: " + strings.Join(parts, ", ")
}

func (r Report) FormatServicesOnly() string {
	var b strings.Builder
	b.WriteString("🛡 <b>Сервисы</b>\n")
	if len(r.Services) == 0 {
		b.WriteString("нет списка")
		return b.String()
	}
	for i, s := range r.Services {
		mark := "❌"
		if s.Active {
			mark = "✅"
		}
		fmt.Fprintf(&b, "%s <code>%s</code> — %s", mark, metrics.Escape(s.Name), metrics.Escape(s.State))
		if i+1 < len(r.Services) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func FormatDockerPS() string {
	if !dockerAvailable() {
		return "❌ Docker недоступен"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "ps", "-a").CombinedOutput()
	if err != nil {
		return "❌ Не удалось выполнить docker ps: " + metrics.Escape(strings.TrimSpace(string(out)))
	}
	body := strings.TrimSpace(string(out))
	if body == "" {
		body = "Контейнеров нет"
	}
	return "🐳 <b>docker ps -a</b>\n<pre>" + EscapePre(body) + "</pre>"
}

// RunScript executes an optional host recovery script (e.g. ensure-vpn.sh).
func RunScript(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("restart script is not configured")
	}
	return runCombined(180*time.Second, path)
}

// EnsureAmnezia keeps the old Amnezia helper for compatibility.
func EnsureAmnezia() (string, error) {
	return RunScript("/usr/local/bin/ensure-amnezia.sh")
}

func RestartContainer(name string) (string, error) {
	return runCombined(60*time.Second, "docker", "restart", name)
}

func runCombined(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, err
	}
	return text, nil
}

func EscapePre(s string) string {
	s = metrics.Escape(s)
	if len(s) > 3500 {
		s = s[:3500] + "…"
	}
	return s
}

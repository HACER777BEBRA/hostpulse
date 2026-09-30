package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID", "CONFIG_FILE",
		"REPORT_INTERVAL", "CHECK_INTERVAL", "REPORT_STYLE",
		"CPU_ALERT", "RAM_ALERT", "DISK_ALERT", "LOAD_ALERT",
		"WATCH_SERVICES", "WATCH_CONTAINERS", "STATE_FILE", "DISK_PATH",
		"HOSTNAME", "ALERT_COOLDOWN", "NET_IFACE",
		"RESTART_SCRIPT", "RESTART_COMMAND",
		"TELEGRAM_ALLOWED_USERS", "TELEGRAM_COMMAND_GROUPS", "TELEGRAM_MEMBER_GROUPS",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

func TestLoadRequiresToken(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "missing.yaml"))
	_, err := Load("")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadParsesEnv(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "" +
		"TELEGRAM_BOT_TOKEN=123:abc\n" +
		"TELEGRAM_CHAT_ID=-10042\n" +
		"REPORT_INTERVAL=12h\n" +
		"CHECK_INTERVAL=30s\n" +
		"CPU_ALERT=70\n" +
		"WATCH_SERVICES=docker, ssh, fail2ban\n" +
		"WATCH_CONTAINERS=amnezia-awg2\n" +
		"NET_IFACE=eth0\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", filepath.Join(dir, "no-such.yaml"))
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BotToken != "123:abc" || cfg.ChatID != -10042 {
		t.Fatalf("token/chat: %+v", cfg)
	}
	if cfg.ReportInterval != 12*time.Hour || cfg.CheckInterval != 30*time.Second {
		t.Fatalf("intervals: %+v", cfg)
	}
	if cfg.CPUAlert != 70 {
		t.Fatalf("cpu alert: %v", cfg.CPUAlert)
	}
	if len(cfg.WatchServices) != 3 || cfg.WatchServices[1] != "ssh" {
		t.Fatalf("services: %#v", cfg.WatchServices)
	}
	if cfg.NetIface != "eth0" {
		t.Fatalf("iface: %s", cfg.NetIface)
	}
	if cfg.RestartCommand != "restart" {
		t.Fatalf("restart cmd: %s", cfg.RestartCommand)
	}
}

func TestLoadYAMLLists(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TELEGRAM_BOT_TOKEN=t\nTELEGRAM_CHAT_ID=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(dir, "config.yaml")
	yamlBody := `
watch:
  services:
    - docker
    - ssh
  containers:
    - nginx
    - postgres
restart:
  command: restart_vpn
  script: /usr/local/bin/fix-vpn.sh
alerts:
  cpu: 75
`
	if err := os.WriteFile(yamlPath, []byte(yamlBody), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WatchContainers) != 2 || cfg.WatchContainers[1] != "postgres" {
		t.Fatalf("containers: %#v", cfg.WatchContainers)
	}
	if cfg.RestartCommand != "restart_vpn" || cfg.RestartScript != "/usr/local/bin/fix-vpn.sh" {
		t.Fatalf("restart: %+v", cfg)
	}
	if cfg.CPUAlert != 75 {
		t.Fatalf("cpu: %v", cfg.CPUAlert)
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "TELEGRAM_BOT_TOKEN=t\nTELEGRAM_CHAT_ID=1\nWATCH_CONTAINERS=from-env\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(yamlPath, []byte("watch:\n  containers:\n    - from-yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.WatchContainers) != 1 || cfg.WatchContainers[0] != "from-env" {
		t.Fatalf("want env override, got %#v", cfg.WatchContainers)
	}
}

func TestLoadAccessLists(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	body := "TELEGRAM_BOT_TOKEN=t\nTELEGRAM_CHAT_ID=-1001\nTELEGRAM_ALLOWED_USERS=5,5,0\n"
	if err := os.WriteFile(envPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(dir, "config.yaml")
	yamlBody := `
access:
  users: [9]
  command_groups: [-1003]
  member_groups: [-1002, -1002]
`
	if err := os.WriteFile(yamlPath, []byte(yamlBody), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllowedUsers) != 1 || cfg.AllowedUsers[0] != 5 {
		t.Fatalf("env users override yaml: %#v", cfg.AllowedUsers)
	}
	if len(cfg.CommandGroups) != 1 || cfg.CommandGroups[0] != -1003 {
		t.Fatalf("command groups: %#v", cfg.CommandGroups)
	}
	if len(cfg.MemberGroups) != 1 || cfg.MemberGroups[0] != -1002 {
		t.Fatalf("member groups: %#v", cfg.MemberGroups)
	}
	if cfg.ChatID != -1001 {
		t.Fatalf("chat id changed: %d", cfg.ChatID)
	}
}

func TestCsvEnvEmpty(t *testing.T) {
	t.Setenv("WATCH_SERVICES", "  ")
	if got := csvEnv("WATCH_SERVICES"); got != nil {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadClampsTinyCheckInterval(t *testing.T) {
	clearConfigEnv(t)
	dir := t.TempDir()
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "1")
	t.Setenv("CHECK_INTERVAL", "0s")
	t.Setenv("REPORT_INTERVAL", "0s")
	t.Setenv("CONFIG_FILE", filepath.Join(dir, "missing.yaml"))
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CheckInterval != time.Second {
		t.Fatalf("check: %s", cfg.CheckInterval)
	}
	if cfg.ReportInterval != time.Minute {
		t.Fatalf("report: %s", cfg.ReportInterval)
	}
}

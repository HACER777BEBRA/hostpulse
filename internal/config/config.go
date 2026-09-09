package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	BotToken        string
	ChatID          int64
	ReportInterval  time.Duration
	CheckInterval   time.Duration
	CPUAlert        float64
	RAMAlert        float64
	DiskAlert       float64
	LoadAlert       float64
	WatchServices   []string
	WatchContainers []string
	StateFile       string
	DiskPath        string
	Hostname        string
	AlertCooldown   time.Duration
	NetIface        string
	ReportStyle     string
	// Optional host script run before docker restart (empty = skip).
	RestartScript string
	// Telegram command without leading slash, e.g. "restart" or "restart_amnezia".
	RestartCommand string
}

type fileConfig struct {
	Telegram struct {
		BotToken string `yaml:"bot_token"`
		ChatID   int64  `yaml:"chat_id"`
	} `yaml:"telegram"`

	ReportInterval string `yaml:"report_interval"`
	CheckInterval  string `yaml:"check_interval"`
	ReportStyle    string `yaml:"report_style"`

	Alerts struct {
		CPU      *float64 `yaml:"cpu"`
		RAM      *float64 `yaml:"ram"`
		Disk     *float64 `yaml:"disk"`
		Load     *float64 `yaml:"load"`
		Cooldown string   `yaml:"cooldown"`
	} `yaml:"alerts"`

	Watch struct {
		Services   []string `yaml:"services"`
		Containers []string `yaml:"containers"`
	} `yaml:"watch"`

	Restart struct {
		Command string `yaml:"command"`
		Script  string `yaml:"script"`
	} `yaml:"restart"`

	Paths struct {
		StateFile string `yaml:"state_file"`
		Disk      string `yaml:"disk"`
	} `yaml:"paths"`

	Hostname string `yaml:"hostname"`
	NetIface string `yaml:"net_iface"`
}

// Load reads .env (secrets) and optional config.yaml (lists/thresholds).
// Env vars always win over YAML so you can override on another host without editing files.
// An explicit envPath is applied with Overload so the file is the source of truth for that run.
func Load(envPath string) (Config, error) {
	if envPath != "" {
		_ = godotenv.Overload(envPath)
	} else {
		_ = godotenv.Load()
	}

	cfg := Config{
		BotToken:        strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		ReportInterval:  durEnv("REPORT_INTERVAL", 24*time.Hour),
		CheckInterval:   durEnv("CHECK_INTERVAL", 60*time.Second),
		CPUAlert:        floatEnv("CPU_ALERT", 85),
		RAMAlert:        floatEnv("RAM_ALERT", 90),
		DiskAlert:       floatEnv("DISK_ALERT", 90),
		LoadAlert:       floatEnv("LOAD_ALERT", 2.0),
		WatchServices:   csvEnv("WATCH_SERVICES"),
		WatchContainers: csvEnv("WATCH_CONTAINERS"),
		StateFile:       strEnv("STATE_FILE", "/var/lib/hostpulse/state.json"),
		DiskPath:        strEnv("DISK_PATH", "/"),
		Hostname:        strEnv("HOSTNAME", ""),
		AlertCooldown:   durEnv("ALERT_COOLDOWN", 15*time.Minute),
		NetIface:        strEnv("NET_IFACE", "auto"),
		ReportStyle:     strEnv("REPORT_STYLE", "full"),
		RestartScript:   strEnv("RESTART_SCRIPT", ""),
		RestartCommand:  normalizeRestartCommand(strEnv("RESTART_COMMAND", "restart")),
	}

	if yc, path, err := loadYAML(envPath); err != nil {
		return Config{}, fmt.Errorf("config file %s: %w", path, err)
	} else if path != "" {
		applyYAML(&cfg, yc)
	}

	// Re-apply env after YAML so machine-specific overrides win.
	overlayEnv(&cfg)

	if cfg.CheckInterval < time.Second {
		cfg.CheckInterval = time.Second
	}
	if cfg.ReportInterval < time.Minute {
		cfg.ReportInterval = time.Minute
	}
	if cfg.AlertCooldown < 0 {
		cfg.AlertCooldown = 0
	}
	if cfg.RestartCommand == "" {
		cfg.RestartCommand = "restart"
	}

	chat := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if chat == "" && cfg.ChatID != 0 {
		chat = strconv.FormatInt(cfg.ChatID, 10)
	}
	if cfg.BotToken == "" || chat == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required (set in .env or config.yaml)")
	}
	id, err := strconv.ParseInt(chat, 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("TELEGRAM_CHAT_ID: %w", err)
	}
	cfg.ChatID = id
	return cfg, nil
}

func loadYAML(envPath string) (fileConfig, string, error) {
	candidates := []string{}
	if v := strings.TrimSpace(os.Getenv("CONFIG_FILE")); v != "" {
		candidates = append(candidates, v)
	}
	if envPath != "" {
		dir := filepath.Dir(envPath)
		candidates = append(candidates,
			filepath.Join(dir, "config.yaml"),
			filepath.Join(dir, "config.yml"),
		)
	}
	candidates = append(candidates, "config.yaml", "config.yml")

	seen := map[string]bool{}
	for _, p := range candidates {
		if p == "" || p == "." || seen[p] {
			continue
		}
		seen[p] = true
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fileConfig{}, p, err
		}
		var yc fileConfig
		if err := yaml.Unmarshal(data, &yc); err != nil {
			return fileConfig{}, p, err
		}
		return yc, p, nil
	}
	return fileConfig{}, "", nil
}

func applyYAML(cfg *Config, yc fileConfig) {
	if yc.Telegram.BotToken != "" && cfg.BotToken == "" {
		cfg.BotToken = strings.TrimSpace(yc.Telegram.BotToken)
	}
	if yc.Telegram.ChatID != 0 && cfg.ChatID == 0 {
		cfg.ChatID = yc.Telegram.ChatID
	}
	if d, ok := parseDur(yc.ReportInterval); ok {
		cfg.ReportInterval = d
	}
	if d, ok := parseDur(yc.CheckInterval); ok {
		cfg.CheckInterval = d
	}
	if yc.ReportStyle != "" {
		cfg.ReportStyle = strings.TrimSpace(yc.ReportStyle)
	}
	if yc.Alerts.CPU != nil {
		cfg.CPUAlert = *yc.Alerts.CPU
	}
	if yc.Alerts.RAM != nil {
		cfg.RAMAlert = *yc.Alerts.RAM
	}
	if yc.Alerts.Disk != nil {
		cfg.DiskAlert = *yc.Alerts.Disk
	}
	if yc.Alerts.Load != nil {
		cfg.LoadAlert = *yc.Alerts.Load
	}
	if d, ok := parseDur(yc.Alerts.Cooldown); ok {
		cfg.AlertCooldown = d
	}
	if len(yc.Watch.Services) > 0 {
		cfg.WatchServices = cleanList(yc.Watch.Services)
	}
	if len(yc.Watch.Containers) > 0 {
		cfg.WatchContainers = cleanList(yc.Watch.Containers)
	}
	if yc.Restart.Script != "" {
		cfg.RestartScript = strings.TrimSpace(yc.Restart.Script)
	}
	if yc.Restart.Command != "" {
		cfg.RestartCommand = normalizeRestartCommand(yc.Restart.Command)
	}
	if yc.Paths.StateFile != "" {
		cfg.StateFile = strings.TrimSpace(yc.Paths.StateFile)
	}
	if yc.Paths.Disk != "" {
		cfg.DiskPath = strings.TrimSpace(yc.Paths.Disk)
	}
	if yc.Hostname != "" {
		cfg.Hostname = strings.TrimSpace(yc.Hostname)
	}
	if yc.NetIface != "" {
		cfg.NetIface = strings.TrimSpace(yc.NetIface)
	}
}

func overlayEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); v != "" {
		cfg.BotToken = v
	}
	if v := strings.TrimSpace(os.Getenv("REPORT_INTERVAL")); v != "" {
		if d, ok := parseDur(v); ok {
			cfg.ReportInterval = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("CHECK_INTERVAL")); v != "" {
		if d, ok := parseDur(v); ok {
			cfg.CheckInterval = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("REPORT_STYLE")); v != "" {
		cfg.ReportStyle = v
	}
	if v := strings.TrimSpace(os.Getenv("CPU_ALERT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.CPUAlert = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAM_ALERT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RAMAlert = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("DISK_ALERT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.DiskAlert = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("LOAD_ALERT")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.LoadAlert = f
		}
	}
	if v := strings.TrimSpace(os.Getenv("WATCH_SERVICES")); v != "" {
		cfg.WatchServices = csvEnv("WATCH_SERVICES")
	}
	if v := strings.TrimSpace(os.Getenv("WATCH_CONTAINERS")); v != "" {
		cfg.WatchContainers = csvEnv("WATCH_CONTAINERS")
	}
	if v := strings.TrimSpace(os.Getenv("STATE_FILE")); v != "" {
		cfg.StateFile = v
	}
	if v := strings.TrimSpace(os.Getenv("DISK_PATH")); v != "" {
		cfg.DiskPath = v
	}
	if v := strings.TrimSpace(os.Getenv("HOSTNAME")); v != "" {
		cfg.Hostname = v
	}
	if v := strings.TrimSpace(os.Getenv("ALERT_COOLDOWN")); v != "" {
		if d, ok := parseDur(v); ok {
			cfg.AlertCooldown = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("NET_IFACE")); v != "" {
		cfg.NetIface = v
	}
	if v := strings.TrimSpace(os.Getenv("RESTART_SCRIPT")); v != "" {
		cfg.RestartScript = v
	}
	if v := strings.TrimSpace(os.Getenv("RESTART_COMMAND")); v != "" {
		cfg.RestartCommand = normalizeRestartCommand(v)
	}
}

func normalizeRestartCommand(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "/")
	s = strings.ToLower(s)
	return s
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseDur(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, false
	}
	return d, true
}

func strEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func floatEnv(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func durEnv(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func csvEnv(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	return cleanList(parts)
}

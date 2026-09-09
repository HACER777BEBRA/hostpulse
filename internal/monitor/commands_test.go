package monitor

import (
	"strings"
	"testing"
	"time"

	"hostpulse/internal/config"
	"hostpulse/internal/netload"
	"hostpulse/internal/reboot"
	"hostpulse/internal/state"
	"hostpulse/internal/telegram"
)

type fakeTG struct {
	sent    []string
	allowed int64
	updates []telegram.Update
	sendErr error
}

func (f *fakeTG) Send(text string) error {
	f.sent = append(f.sent, text)
	return f.sendErr
}
func (f *fakeTG) GetUpdates(offset int) ([]telegram.Update, error) {
	return f.updates, nil
}
func (f *fakeTG) AllowedChat(id int64) bool { return id == f.allowed }

func newTestApp(t *testing.T, tg *fakeTG) *App {
	t.Helper()
	st, err := state.Load(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		ChatID:         42,
		CheckInterval:  time.Minute,
		ReportInterval: 24 * time.Hour,
		AlertCooldown:  15 * time.Minute,
		WatchServices:  []string{"ssh"},
		DiskPath:       "/",
		Hostname:       "amneziavpn.aeza.network",
	}
	tg.allowed = 42
	return New(cfg, tg, st, netload.New("net0"))
}

func TestHelpMediumAndFull(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	app.HandleCommand("/help")
	medium := tg.sent[0]
	if !strings.Contains(medium, "средняя справка") || !strings.Contains(medium, "/help full") {
		t.Fatalf("medium: %#v", medium)
	}
	if strings.Contains(medium, "Пороги:") {
		t.Fatal("medium help should not include threshold details")
	}
	app.HandleCommand("/help full")
	full := tg.sent[len(tg.sent)-1]
	if !strings.Contains(full, "развёрнутая справка") || !strings.Contains(full, "/remind short") {
		t.Fatalf("full: %#v", full)
	}
	app.HandleCommand("/help_full")
	if !strings.Contains(tg.sent[len(tg.sent)-1], "развёрнутая справка") {
		t.Fatalf("help_full: %#v", tg.sent[len(tg.sent)-1])
	}
	app.HandleCommand("/help nope")
	if !strings.Contains(tg.sent[len(tg.sent)-1], "/help full") {
		t.Fatalf("bad help arg: %#v", tg.sent[len(tg.sent)-1])
	}
}

func TestRemindStyle(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	if app.reportStyle() != reportStyleFull {
		t.Fatalf("default %s", app.reportStyle())
	}
	app.HandleCommand("/remind")
	if !strings.Contains(tg.sent[0], "развёрнутый") {
		t.Fatalf("show: %#v", tg.sent[0])
	}
	app.HandleCommand("/remind short")
	if app.reportStyle() != reportStyleShort {
		t.Fatalf("got %s", app.reportStyle())
	}
	if !strings.Contains(tg.sent[len(tg.sent)-1], "краткий") {
		t.Fatalf("set short: %#v", tg.sent[len(tg.sent)-1])
	}
	app.HandleCommand("/remind краткая")
	if app.reportStyle() != reportStyleShort {
		t.Fatal("russian alias")
	}
	app.HandleCommand("/remind full")
	if app.reportStyle() != reportStyleFull {
		t.Fatalf("back to full: %s", app.reportStyle())
	}
	app.HandleCommand("/remind forever")
	if !strings.Contains(tg.sent[len(tg.sent)-1], "/remind short") {
		t.Fatalf("bad: %#v", tg.sent[len(tg.sent)-1])
	}
}

func TestParseHelpAndReportStyle(t *testing.T) {
	if got, ok := parseHelpStyle("развёрнутая"); !ok || got != helpStyleFull {
		t.Fatalf("help full: %v %v", got, ok)
	}
	if got, ok := parseHelpStyle("medium"); !ok || got != helpStyleMedium {
		t.Fatalf("help medium: %v %v", got, ok)
	}
	if _, ok := parseHelpStyle("nope"); ok {
		t.Fatal("expected reject")
	}
	if got, ok := parseReportStyle("короткая"); !ok || got != reportStyleShort {
		t.Fatalf("short: %s %v", got, ok)
	}
	if got, ok := parseReportStyle("полная"); !ok || got != reportStyleFull {
		t.Fatalf("full: %s %v", got, ok)
	}
}

func TestHelpAndUnknown(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	app.HandleCommand("/help")
	if len(tg.sent) != 1 || !strings.Contains(tg.sent[0], "/interval") {
		t.Fatalf("help: %#v", tg.sent)
	}
	app.HandleCommand("/nope")
	if tg.sent[len(tg.sent)-1] != "Неизвестная команда. /help или /help full" {
		t.Fatalf("unknown: %#v", tg.sent)
	}
}

func TestMuteUnmute(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	app.HandleCommand("/mute 1h")
	if !strings.Contains(tg.sent[0], "Алерты выключены") {
		t.Fatalf("mute: %#v", tg.sent)
	}
	if !app.store.IsMuted(time.Now()) {
		t.Fatal("expected muted")
	}
	app.HandleCommand("/unmute")
	if tg.sent[len(tg.sent)-1] != "🔔 Алерты снова включены" {
		t.Fatalf("unmute: %#v", tg.sent)
	}
}

func TestMuteBadDuration(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	app.HandleCommand("/mute forever")
	if !strings.Contains(tg.sent[0], "/mute 1h") {
		t.Fatalf("got %#v", tg.sent)
	}
}

func TestFormatReboot(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	prev := state.File{
		BootID:        "old",
		LastBootTime:  time.Date(2026, 8, 25, 22, 57, 3, 0, time.UTC),
		LastSeenAlive: time.Date(2026, 8, 28, 2, 17, 13, 0, time.UTC),
	}
	info := reboot.Info{
		BootID:   "0edbb020-aaaa-bbbb-cccc-dddddddddddd",
		BootTime: time.Date(2026, 8, 28, 2, 25, 39, 0, time.UTC),
	}
	got := app.formatReboot(prev, info)
	for _, n := range []string{
		"Сервер перезапустился",
		"2026-08-25 22:57:03 UTC",
		"2026-08-28 02:25:39 UTC",
		"0edbb020…",
		"02:17:13",
		"02:25:39 UTC",
	} {
		if !strings.Contains(got, n) {
			t.Fatalf("missing %q in %s", n, got)
		}
	}
}

func TestCommandAtBotSuffix(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	app.HandleCommand("/help@MyMonitorBot")
	if !strings.Contains(tg.sent[0], "Команды") {
		t.Fatalf("got %#v", tg.sent)
	}
}

func TestIntervalCommand(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	app.HandleCommand("/interval")
	if !strings.Contains(tg.sent[0], "24h") {
		t.Fatalf("show current: %#v", tg.sent)
	}
	app.HandleCommand("/interval 48h")
	if !strings.Contains(tg.sent[len(tg.sent)-1], "48h") {
		t.Fatalf("set 48h: %#v", tg.sent)
	}
	if app.reportInterval() != 48*time.Hour {
		t.Fatalf("got %s", app.reportInterval())
	}
	app.HandleCommand("/interval 12")
	if app.reportInterval() != 12*time.Hour {
		t.Fatalf("bare hours: %s", app.reportInterval())
	}
	app.HandleCommand("/interval forever")
	if !strings.Contains(tg.sent[len(tg.sent)-1], "Укажите интервал") {
		t.Fatalf("bad: %#v", tg.sent)
	}
}

func TestParseInterval(t *testing.T) {
	cases := map[string]time.Duration{
		"24h":   24 * time.Hour,
		"48h":   48 * time.Hour,
		"12":    12 * time.Hour,
		"6h":    6 * time.Hour,
		"90m":   90 * time.Minute,
		"48ч":   48 * time.Hour,
		"24час": 24 * time.Hour,
	}
	for raw, want := range cases {
		got, err := parseInterval(raw)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if got != want {
			t.Fatalf("%q: got %s want %s", raw, got, want)
		}
	}
	if _, err := parseInterval("0"); err == nil {
		t.Fatal("expected error for 0")
	}
	if _, err := parseInterval("800h"); err == nil {
		t.Fatal("expected error for >30d")
	}
}

func TestFormatInterval(t *testing.T) {
	if got := formatInterval(48 * time.Hour); got != "48h" {
		t.Fatalf("got %q", got)
	}
	if got := formatInterval(90 * time.Minute); got != "1h30m" {
		t.Fatalf("got %q", got)
	}
}

func TestMaybeAlertMarksOnlyOnSendOK(t *testing.T) {
	tg := &fakeTG{sendErr: errStr("boom")}
	app := newTestApp(t, tg)
	now := time.Now()
	app.maybeAlert("cpu", true, now, "alert")
	if !app.store.CanAlert("cpu", now.Add(time.Second), 15*time.Minute) {
		t.Fatal("failed send must not start cooldown")
	}
	tg.sendErr = nil
	app.maybeAlert("cpu", true, now, "alert")
	if app.store.CanAlert("cpu", now.Add(time.Second), 15*time.Minute) {
		t.Fatal("successful send should start cooldown")
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }

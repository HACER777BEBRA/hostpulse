package monitor

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"hostpulse/internal/config"
	"hostpulse/internal/health"
	"hostpulse/internal/metrics"
	"hostpulse/internal/netload"
	"hostpulse/internal/reboot"
	"hostpulse/internal/state"
	"hostpulse/internal/telegram"
)

type Sender interface {
	Send(text string) error
	SendTo(chatID int64, text string) error
	SendMessage(chatID int64, text string) (int, error)
	EditMessage(chatID int64, messageID int, text string) error
	DeleteMessage(chatID int64, messageID int) error
	GetUpdates(offset int) ([]telegram.Update, error)
	AllowedChat(id int64) bool
	IsMember(chatID, userID int64) (bool, error)
}

type App struct {
	cfg           config.Config
	tg            Sender
	store         *state.Store
	net           *netload.Sampler
	logf          func(string, ...any)
	intervalReset chan struct{}
	// replyTo is the chat that asked for the command currently running.
	// Only the poll goroutine reads and writes it.
	replyTo int64
	baseCtx context.Context
	liveMu  sync.Mutex
	lives   map[int64]*liveRun
}

func New(cfg config.Config, tg Sender, store *state.Store, net *netload.Sampler) *App {
	return &App{
		cfg:           cfg,
		tg:            tg,
		store:         store,
		net:           net,
		logf:          log.Printf,
		intervalReset: make(chan struct{}, 1),
	}
}

func (a *App) Start(ctx context.Context) {
	stop := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(stop)
	}()
	go a.net.Start(stop)

	info, err := reboot.Detect()
	prev := a.store.Snapshot()
	if err == nil {
		if prev.BootID != "" && prev.BootID != info.BootID {
			a.send(a.formatReboot(prev, info))
		}
		if err := a.store.SetBoot(info.BootID, info.BootTime); err != nil {
			a.logf("set boot: %v", err)
		}
	}
	if err := a.store.TouchAlive(); err != nil {
		a.logf("touch alive: %v", err)
	}
	if a.store.LastReportAt().IsZero() {
		if err := a.store.SetLastReportAt(time.Now().UTC()); err != nil {
			a.logf("set last report: %v", err)
		}
	}
	a.send(a.formatStarted(info))
}

func (a *App) OnStop() {
	a.send("🔴 Монитор остановлен")
}

func (a *App) Run(ctx context.Context) {
	a.baseCtx = ctx
	defer a.stopLive()
	a.Start(ctx)
	check := time.NewTicker(a.cfg.CheckInterval)
	defer check.Stop()

	report := time.NewTimer(a.timeUntilReport())
	defer report.Stop()

	go a.pollCommands(ctx)

	resetReportTimer := func() {
		if !report.Stop() {
			select {
			case <-report.C:
			default:
			}
		}
		report.Reset(a.timeUntilReport())
	}

	for {
		select {
		case <-ctx.Done():
			a.stopLive()
			a.OnStop()
			return
		case <-check.C:
			a.RunChecks()
		case <-report.C:
			if a.SendReport() {
				if err := a.store.SetLastReportAt(time.Now().UTC()); err != nil {
					a.logf("set last report: %v", err)
				}
				report.Reset(a.reportInterval())
			} else {
				report.Reset(time.Minute)
			}
		case <-a.intervalReset:
			resetReportTimer()
		}
	}
}

func (a *App) pollCommands(ctx context.Context) {
	offset := 0
	for {
		if ctx.Err() != nil {
			return
		}
		updates, err := a.tg.GetUpdates(offset)
		if err != nil {
			a.logf("getUpdates: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || strings.TrimSpace(u.Message.Text) == "" {
				continue
			}
			userID := int64(0)
			if u.Message.From != nil {
				userID = u.Message.From.ID
			}
			if !a.allowCommand(u.Message.Chat.ID, userID) {
				continue
			}
			a.replyTo = u.Message.Chat.ID
			a.HandleCommand(strings.TrimSpace(u.Message.Text))
			a.replyTo = 0
		}
	}
}

func (a *App) RunChecks() {
	if err := a.store.TouchAlive(); err != nil {
		a.logf("touch alive: %v", err)
	}
	snap := metrics.Collect(a.cfg.Hostname, a.cfg.DiskPath, false)
	now := time.Now()
	if snap.CPUOK {
		a.maybeAlert("cpu", snap.CPUPercent >= a.cfg.CPUAlert, now,
			fmt.Sprintf("⚠️ <b>Высокий CPU</b>: %.1f%% (порог %.0f%%)", snap.CPUPercent, a.cfg.CPUAlert))
	}
	if snap.MemOK {
		a.maybeAlert("ram", snap.MemPercent >= a.cfg.RAMAlert, now,
			fmt.Sprintf("⚠️ <b>Высокая RAM</b>: %.1f%% (порог %.0f%%)", snap.MemPercent, a.cfg.RAMAlert))
	}
	if snap.DiskOK {
		a.maybeAlert("disk", snap.DiskPercent >= a.cfg.DiskAlert, now,
			fmt.Sprintf("⚠️ <b>Мало места на диске</b>: %.1f%% занято, свободно %s", snap.DiskPercent, metrics.HumanBytes(snap.DiskFree)))
	}
	if snap.LoadOK {
		a.maybeAlert("load", snap.Load1 >= a.cfg.LoadAlert, now,
			fmt.Sprintf("⚠️ <b>Высокий load average</b>: %.2f (порог %.2f)", snap.Load1, a.cfg.LoadAlert))
	}

	rep := health.Check(a.cfg.WatchServices, a.cfg.WatchContainers)
	for _, svc := range rep.Services {
		a.maybeAlert("svc:"+svc.Name, !svc.Active, now,
			fmt.Sprintf("❌ Сервис <code>%s</code> не активен (%s)", metrics.Escape(svc.Name), metrics.Escape(svc.State)))
	}
	for _, c := range rep.Containers {
		a.maybeAlert("ctr:"+c.Name, !c.Running, now,
			fmt.Sprintf("❌ Контейнер <code>%s</code>: %s", metrics.Escape(c.Name), metrics.Escape(c.State)))
	}
}

func (a *App) maybeAlert(key string, firing bool, now time.Time, text string) {
	if !firing {
		if err := a.store.ClearAlert(key); err != nil {
			a.logf("clear alert: %v", err)
		}
		return
	}
	if !a.store.CanAlert(key, now, a.cfg.AlertCooldown) {
		return
	}
	if !a.send(text) {
		return
	}
	if err := a.store.MarkAlert(key, now); err != nil {
		a.logf("mark alert: %v", err)
	}
}

func (a *App) SendReport() bool {
	if a.reportStyle() == reportStyleShort {
		return a.send("📊 <b>Краткий отчёт</b>\n\n" + a.statusCompact())
	}
	return a.send("📊 <b>Отчёт о сервере</b>\n\n" + a.statusWithNetServices(true))
}

// allowCommand decides who may run a command and where the reply goes.
// Member groups: any participant is answered in that group and in DM.
// Command groups: only the whitelist (explicit user ids and members of
// member groups). Other groups are ignored. A stranger in DM gets a refusal.
func (a *App) allowCommand(chatID, userID int64) bool {
	if a.inMemberGroup(chatID) {
		return true
	}
	if containsID(a.cfg.CommandGroups, chatID) {
		ok, err := a.whitelisted(userID)
		if err != nil {
			a.logf("whitelist: %v", err)
			return false
		}
		return ok
	}
	if chatID > 0 {
		if !a.accessConfigured() && a.tg.AllowedChat(chatID) {
			return true
		}
		ok, err := a.whitelisted(userID)
		a.replyTo = chatID
		defer func() { a.replyTo = 0 }()
		if err != nil {
			a.logf("whitelist: %v", err)
			a.reply("Не удалось проверить доступ. Повторите команду.")
			return false
		}
		if !ok {
			a.reply("Команды в личке доступны только из белого списка.")
			return false
		}
		return true
	}
	return false
}

func (a *App) accessConfigured() bool {
	return len(a.cfg.AllowedUsers) > 0 || len(a.cfg.CommandGroups) > 0 || len(a.cfg.MemberGroups) > 0
}

func (a *App) effectiveMemberGroups() []int64 {
	if a.accessConfigured() {
		return a.cfg.MemberGroups
	}
	if a.cfg.ChatID < 0 {
		return []int64{a.cfg.ChatID}
	}
	return nil
}

func (a *App) inMemberGroup(chatID int64) bool {
	return containsID(a.effectiveMemberGroups(), chatID)
}

func (a *App) whitelisted(userID int64) (bool, error) {
	if userID == 0 {
		return false, nil
	}
	if containsID(a.cfg.AllowedUsers, userID) || (a.cfg.ChatID > 0 && userID == a.cfg.ChatID) {
		return true, nil
	}
	var firstErr error
	for _, group := range a.effectiveMemberGroups() {
		ok, err := a.tg.IsMember(group, userID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ok {
			return true, nil
		}
	}
	if firstErr != nil {
		return false, firstErr
	}
	return false, nil
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (a *App) HandleCommand(text string) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}
	cmd := strings.ToLower(strings.SplitN(parts[0], "@", 2)[0])
	switch cmd {
	case "/help", "/start":
		a.cmdHelp(parts[1:])
	case "/help_full":
		a.cmdHelp([]string{"full"})
	case "/status":
		a.reply(a.statusWithNetServices(false))
	case "/report":
		a.reply("📊 <b>Отчёт о сервере</b>\n\n" + a.statusWithNetServices(true))
	case "/network":
		a.reply(a.net.Snapshot().Format())
	case "/processes":
		a.reply(metrics.Collect(a.cfg.Hostname, a.cfg.DiskPath, true).FormatProcesses())
	case "/docker":
		a.reply(health.FormatDockerPS())
	case "/services":
		a.reply(health.Check(a.cfg.WatchServices, nil).FormatServicesOnly())
	case "/reboot_info":
		a.cmdRebootInfo()
	case "/mute":
		a.cmdMute(parts[1:])
	case "/unmute":
		if err := a.store.Unmute(); err != nil {
			a.reply("❌ " + metrics.Escape(err.Error()))
			return
		}
		a.reply("🔔 Алерты снова включены")
	case "/interval":
		a.cmdInterval(parts[1:])
	case "/remind":
		a.cmdRemind(parts[1:])
	case "/live":
		a.cmdLive(parts[1:])
	default:
		if a.isRestartCommand(cmd) {
			a.cmdRestart()
			return
		}
		a.reply("Неизвестная команда. /help или /help full")
	}
}

func (a *App) isRestartCommand(cmd string) bool {
	name := strings.TrimPrefix(cmd, "/")
	want := a.cfg.RestartCommand
	if want == "" {
		want = "restart"
	}
	if name == want {
		return true
	}
	// Back-compat with older bots / muscle memory.
	return name == "restart_amnezia" || name == "restart"
}

func (a *App) cmdRebootInfo() {
	info, err := reboot.Detect()
	st := a.store.Snapshot()
	var b strings.Builder
	if err != nil {
		fmt.Fprintf(&b, "♻️ Boot: <code>%s</code>\n", metrics.Escape(err.Error()))
	} else {
		fmt.Fprintf(&b, "♻️ Boot: <code>%s</code>\n", info.BootTime.UTC().Format("2006-01-02 15:04:05 UTC"))
		fmt.Fprintf(&b, "⏱ Uptime: <code>%s</code>\n", metrics.FormatDuration(info.Uptime))
	}
	id := st.BootID
	if id == "" && err == nil {
		id = info.BootID
	}
	fmt.Fprintf(&b, "🆔 BootID в state: <code>%s</code>", metrics.Escape(id))
	a.reply(b.String())
}

func (a *App) cmdRestart() {
	if script := strings.TrimSpace(a.cfg.RestartScript); script != "" {
		out, err := health.RunScript(script)
		label := filepathBase(script)
		if err != nil {
			a.reply("❌ " + metrics.Escape(label) + ": " + metrics.Escape(err.Error()))
		} else {
			body := strings.TrimSpace(out)
			if body == "" {
				body = "OK"
			}
			a.reply("✅ " + metrics.Escape(label) + ":\n<pre>" + health.EscapePre(body) + "</pre>")
		}
	}
	if len(a.cfg.WatchContainers) == 0 && strings.TrimSpace(a.cfg.RestartScript) == "" {
		a.reply("Нечего перезапускать: задайте <code>watch.containers</code> и/или <code>restart.script</code> в config.yaml")
		return
	}
	for _, name := range a.cfg.WatchContainers {
		out, err := health.RestartContainer(name)
		if err != nil {
			a.reply("❌ docker restart <code>" + metrics.Escape(name) + "</code>: " + metrics.Escape(err.Error()))
			continue
		}
		body := strings.TrimSpace(out)
		if body == "" {
			body = name
		}
		a.reply("✅ Контейнер перезапущен:\n<pre>" + health.EscapePre(body) + "</pre>")
	}
}

func filepathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (a *App) cmdMute(args []string) {
	raw := "1h"
	if len(args) > 0 {
		raw = args[0]
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		a.reply("Укажите длительность: <code>/mute 1h</code>, <code>/mute 30m</code>")
		return
	}
	if err := a.store.Mute(time.Now().Add(d)); err != nil {
		a.reply("❌ " + metrics.Escape(err.Error()))
		return
	}
	a.reply(fmt.Sprintf("🔇 Алерты выключены на %s", d.Round(time.Second)))
}

func (a *App) cmdInterval(args []string) {
	if len(args) == 0 {
		d := a.reportInterval()
		a.reply(fmt.Sprintf(
			"⏱ Периодический отчёт: каждые <code>%s</code>\nСледующий примерно через <code>%s</code>\n\nЧтобы сменить: <code>/interval 24h</code>, <code>/interval 48h</code> или <code>/interval 12</code> (часы).",
			formatInterval(d), formatInterval(a.timeUntilReport()),
		))
		return
	}
	d, err := parseInterval(args[0])
	if err != nil {
		a.reply("Укажите интервал: <code>/interval 24h</code>, <code>/interval 48h</code> или число часов, например <code>/interval 12</code>")
		return
	}
	if err := a.store.SetReportInterval(d); err != nil {
		a.reply("❌ " + metrics.Escape(err.Error()))
		return
	}
	if err := a.store.SetLastReportAt(time.Now().UTC()); err != nil {
		a.reply("❌ " + metrics.Escape(err.Error()))
		return
	}
	a.notifyIntervalReset()
	a.reply(fmt.Sprintf("⏱ Периодический отчёт теперь каждые <code>%s</code>\nСледующий — через <code>%s</code>", formatInterval(d), formatInterval(d)))
}

func (a *App) reportInterval() time.Duration {
	if d := a.store.ReportInterval(); d > 0 {
		return d
	}
	if a.cfg.ReportInterval > 0 {
		return a.cfg.ReportInterval
	}
	return 24 * time.Hour
}

func (a *App) timeUntilReport() time.Duration {
	interval := a.reportInterval()
	last := a.store.LastReportAt()
	if last.IsZero() {
		return interval
	}
	left := interval - time.Since(last)
	if left < time.Second {
		return time.Second
	}
	return left
}

func (a *App) notifyIntervalReset() {
	select {
	case a.intervalReset <- struct{}{}:
	default:
	}
}

func (a *App) statusCompact() string {
	snap := metrics.Collect(a.cfg.Hostname, a.cfg.DiskPath, false)
	rep := health.Check(a.cfg.WatchServices, a.cfg.WatchContainers)
	var b strings.Builder
	b.WriteString(snap.FormatStatusCompact())
	b.WriteString("\n")
	b.WriteString(a.net.Snapshot().FormatCompact())
	b.WriteString("\n")
	b.WriteString(rep.FormatCompact())
	return b.String()
}

func (a *App) statusWithNetServices(withContainers bool) string {
	snap := metrics.Collect(a.cfg.Hostname, a.cfg.DiskPath, false)
	var b strings.Builder
	b.WriteString(snap.FormatStatus())
	b.WriteString("\n\n")
	b.WriteString(a.net.Snapshot().Format())
	b.WriteString("\n\n")
	containers := a.cfg.WatchContainers
	if !withContainers {
		containers = nil
	}
	rep := health.Check(a.cfg.WatchServices, containers)
	if withContainers {
		b.WriteString(rep.Format())
	} else {
		b.WriteString(rep.FormatServicesOnly())
	}
	return b.String()
}

func (a *App) formatStarted(info reboot.Info) string {
	var b strings.Builder
	b.WriteString("🟢 Монитор запущен\n")
	if !info.BootTime.IsZero() {
		fmt.Fprintf(&b, "Boot: <code>%s</code>\n", info.BootTime.UTC().Format("2006-01-02 15:04:05 UTC"))
	}
	if info.BootID != "" {
		fmt.Fprintf(&b, "Boot ID: <code>%s</code>", reboot.Short(info.BootID))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (a *App) formatReboot(prev state.File, info reboot.Info) string {
	var b strings.Builder
	b.WriteString("♻️ <b>Сервер перезапустился</b>\n")
	was := prev.LastBootTime.UTC()
	now := info.BootTime.UTC()
	fmt.Fprintf(&b, "Было: <code>%s</code>\n", was.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(&b, "Стало: <code>%s</code>\n", now.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintf(&b, "Boot ID: <code>%s</code>\n", reboot.Short(info.BootID))
	from := prev.LastSeenAlive.UTC()
	if from.IsZero() {
		from = was
	}
	fmt.Fprintf(&b, "⏱ Примерный простой: <code>%s</code> → <code>%s</code>",
		from.Format("15:04:05"), now.Format("15:04:05 UTC"))
	return b.String()
}

func (a *App) send(text string) bool {
	return a.sendTo(a.cfg.ChatID, text)
}

func (a *App) reply(text string) bool {
	chat := a.replyTo
	if chat == 0 {
		chat = a.cfg.ChatID
	}
	return a.sendTo(chat, text)
}

func (a *App) sendTo(chat int64, text string) bool {
	if strings.TrimSpace(text) == "" {
		return true
	}
	var err error
	if chat == 0 || chat == a.cfg.ChatID {
		err = a.tg.Send(text)
	} else {
		err = a.tg.SendTo(chat, text)
	}
	if err != nil {
		a.logf("send: %v", err)
		return false
	}
	return true
}

func parseInterval(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return clampInterval(d)
	}
	rus := raw
	rus = strings.ReplaceAll(rus, "часов", "h")
	rus = strings.ReplaceAll(rus, "часа", "h")
	rus = strings.ReplaceAll(rus, "час", "h")
	rus = strings.ReplaceAll(rus, "ч", "h")
	if d, err := time.ParseDuration(rus); err == nil && d > 0 {
		return clampInterval(d)
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid interval")
	}
	return clampInterval(time.Duration(n * float64(time.Hour)))
}

func clampInterval(d time.Duration) (time.Duration, error) {
	const min = time.Minute
	const max = 30 * 24 * time.Hour
	if d < min || d > max {
		return 0, fmt.Errorf("out of range")
	}
	return d, nil
}

func formatInterval(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d >= time.Minute && d%time.Minute == 0 {
		h := int(d / time.Hour)
		m := int(d/time.Minute) % 60
		if h > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dm", m)
	}
	return d.String()
}

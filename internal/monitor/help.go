package monitor

import (
	"fmt"
	"strings"

	"hostpulse/internal/metrics"
)

const (
	reportStyleFull  = "full"
	reportStyleShort = "short"
)

func (a *App) cmdHelp(args []string) {
	style := helpStyleMedium
	if len(args) > 0 {
		parsed, ok := parseHelpStyle(args[0])
		if !ok {
			a.send("Справка: <code>/help</code> (средняя) или <code>/help full</code> (развёрнутая)")
			return
		}
		style = parsed
	}
	if style == helpStyleFull {
		a.send(a.helpTextFull())
		return
	}
	a.send(a.helpTextMedium())
}

func (a *App) cmdRemind(args []string) {
	if len(args) == 0 {
		a.send(fmt.Sprintf(
			"📬 Автонапоминание: <b>%s</b> формат каждые <code>%s</code>\n\nПереключить: <code>/remind short</code> или <code>/remind full</code>",
			reportStyleName(a.reportStyle()), formatInterval(a.reportInterval()),
		))
		return
	}
	style, ok := parseReportStyle(args[0])
	if !ok {
		a.send("Укажите формат: <code>/remind short</code> (краткая) или <code>/remind full</code> (развёрнутая)")
		return
	}
	if err := a.store.SetReportStyle(style); err != nil {
		a.send("❌ " + metrics.Escape(err.Error()))
		return
	}
	a.send(fmt.Sprintf("📬 Автонапоминание переключено на <b>%s</b> формат (каждые <code>%s</code>)",
		reportStyleName(style), formatInterval(a.reportInterval())))
}

func (a *App) reportStyle() string {
	if s := a.store.ReportStyle(); s != "" {
		if parsed, ok := parseReportStyle(s); ok {
			return parsed
		}
	}
	if parsed, ok := parseReportStyle(a.cfg.ReportStyle); ok {
		return parsed
	}
	return reportStyleFull
}

type helpStyle int

const (
	helpStyleMedium helpStyle = iota
	helpStyleFull
)

func parseHelpStyle(raw string) (helpStyle, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "full", "long", "detailed", "развернутая", "развёрнутая", "полная", "подробно":
		return helpStyleFull, true
	case "medium", "mid", "средняя", "средняясправка":
		return helpStyleMedium, true
	default:
		return 0, false
	}
}

func parseReportStyle(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case reportStyleFull, "long", "detailed", "развернутая", "развёрнутая", "полная", "полный":
		return reportStyleFull, true
	case reportStyleShort, "brief", "compact", "краткая", "краткий", "короткая", "короткий":
		return reportStyleShort, true
	default:
		return "", false
	}
}

func reportStyleName(style string) string {
	if style == reportStyleShort {
		return "краткий"
	}
	return "развёрнутый"
}

func (a *App) helpTextMedium() string {
	return strings.TrimSpace(fmt.Sprintf(`🤖 <b>HostPulse</b> — средняя справка

<b>Команды:</b>
/status — CPU, RAM, диск, сеть, сервисы
/network — нагрузка сети ↓↑
/processes — топ процессов
/docker — список контейнеров
/services — статусы сервисов
/reboot_info — загрузка и uptime
/report — полный отчёт сейчас
/interval %s — частота автоотчётов
/remind — формат автонапоминания (сейчас %s)
/%s — скрипт восстановления + docker restart
/mute 1h — выключить алерты
/unmute — включить алерты
/help — эта справка
/help full — развёрнутая справка

Авто: отчёты в %s формате, алерты, уведомление о ребуте`,
		formatInterval(a.reportInterval()),
		reportStyleName(a.reportStyle()),
		a.restartCommandName(),
		reportStyleName(a.reportStyle()),
	))
}

func (a *App) restartCommandName() string {
	if a.cfg.RestartCommand != "" {
		return a.cfg.RestartCommand
	}
	return "restart"
}

func (a *App) helpTextFull() string {
	return strings.TrimSpace(fmt.Sprintf(`🤖 <b>HostPulse</b> — развёрнутая справка

Монитор Linux-хоста в Telegram: метрики, сервисы, Docker, алерты по порогам и периодические напоминания.

<b>Справка</b>
/help — средняя: список команд и текущие режимы
/help full — эта страница с пояснениями
/start — то же, что /help

<b>Снимки состояния</b>
/status — средняя сводка: CPU, RAM, диск, сеть, systemd (без Docker)
/report — развёрнутый отчёт прямо сейчас: метрики, сеть по окнам, сервисы и контейнеры
/network — скорость ↓↑ сейчас / 1 мин / 15 мин / 1 час
/processes — топ процессов по CPU и RAM
/docker — <code>docker ps -a</code>
/services — статусы systemd из WATCH_SERVICES
/reboot_info — boot time, uptime, boot id

<b>Автонапоминания</b>
Периодический отчёт уходит сам, без команды.
Сейчас: каждые <code>%s</code>, формат — <b>%s</b>.
/interval — показать интервал
/interval 24h — сменить (также <code>48h</code> или число часов, например <code>12</code>)
/remind — показать формат автоотчёта
/remind short — краткая справка в автонапоминании
/remind full — развёрнутая справка в автонапоминании

<b>Алерты</b>
Пороги: CPU %.0f%%, RAM %.0f%%, диск %.0f%%, load %.2f.
Повтор одного и того же алерта не чаще чем раз в %s.
/mute 1h — заглушить (также <code>30m</code>)
/unmute — снова включить

<b>Обслуживание</b>
/%s — опциональный скрипт (<code>restart.script</code>) и <code>docker restart</code> для списка <code>watch.containers</code>

Автоматически также приходит уведомление, если сменился boot id (ребут).`,
		formatInterval(a.reportInterval()),
		reportStyleName(a.reportStyle()),
		a.cfg.CPUAlert, a.cfg.RAMAlert, a.cfg.DiskAlert, a.cfg.LoadAlert,
		formatInterval(a.cfg.AlertCooldown),
		a.restartCommandName(),
	))
}

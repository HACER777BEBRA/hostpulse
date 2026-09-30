package monitor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	liveDefaultEvery = 30 * time.Second
	liveMinEvery     = 10 * time.Second
	liveMaxEvery     = time.Hour
	liveTextLimit    = 3900
)

func (a *App) cmdLive(args []string) {
	stop, every, style, err := parseLiveArgs(args, a.reportStyle())
	if err != nil {
		a.reply("Формат: <code>/live</code>, <code>/live short</code>, <code>/live full 15</code>, <code>/live stop</code>\nИнтервал от 10 секунд до 1 часа, по умолчанию 30с.")
		return
	}
	chat := a.replyTo
	if chat == 0 {
		chat = a.cfg.ChatID
	}
	if stop {
		if a.stopLiveChat(chat) {
			a.reply("🛑 Живая сводка остановлена")
			return
		}
		a.reply("Живая сводка не запущена")
		return
	}
	text := a.liveBody(style, every)
	msgID, err := a.tg.SendMessage(chat, text)
	if err != nil {
		a.logf("live send: %v", err)
		a.reply("❌ Не удалось отправить живую сводку")
		return
	}
	a.startLive(chat, msgID, every, style)
}

func (a *App) liveBody(style string, every time.Duration) string {
	var b strings.Builder
	if style == reportStyleShort {
		b.WriteString("📊 <b>Краткий статус</b>\n\n")
		b.WriteString(a.statusCompact())
	} else {
		b.WriteString("📊 <b>Полный статус</b>\n\n")
		b.WriteString(a.statusWithNetServices(true))
	}
	fmt.Fprintf(&b, "\n\n🔄 <i>%s</i> · каждые %s · /live stop",
		time.Now().UTC().Format("15:04:05 UTC"), formatInterval(every))
	return trimTelegram(b.String())
}

type liveRun struct {
	cancel context.CancelFunc
	msgID  int
}

func (a *App) startLive(chat int64, messageID int, every time.Duration, style string) {
	parent := a.baseCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	oldID := a.installLive(chat, messageID, cancel)
	if oldID != 0 {
		a.deleteLiveMessage(chat, oldID)
	}

	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !a.liveCurrent(chat, messageID) {
					return
				}
				err := a.tg.EditMessage(chat, messageID, a.liveBody(style, every))
				if err == nil || liveEditUnchanged(err) {
					continue
				}
				a.logf("live edit: %v", err)
				if liveEditStopped(err) {
					a.dropLive(chat, messageID)
					return
				}
			}
		}
	}()
}

func (a *App) installLive(chat int64, messageID int, cancel context.CancelFunc) int {
	a.liveMu.Lock()
	defer a.liveMu.Unlock()
	if a.lives == nil {
		a.lives = map[int64]*liveRun{}
	}
	oldID := 0
	if prev := a.lives[chat]; prev != nil {
		prev.cancel()
		oldID = prev.msgID
	}
	a.lives[chat] = &liveRun{cancel: cancel, msgID: messageID}
	return oldID
}

func (a *App) liveCurrent(chat int64, messageID int) bool {
	a.liveMu.Lock()
	defer a.liveMu.Unlock()
	run := a.lives[chat]
	return run != nil && run.msgID == messageID
}

func (a *App) dropLive(chat int64, messageID int) {
	a.liveMu.Lock()
	defer a.liveMu.Unlock()
	run := a.lives[chat]
	if run == nil || run.msgID != messageID {
		return
	}
	run.cancel()
	delete(a.lives, chat)
}

func (a *App) stopLiveChat(chat int64) bool {
	a.liveMu.Lock()
	run := a.lives[chat]
	if run == nil {
		a.liveMu.Unlock()
		return false
	}
	msgID := run.msgID
	run.cancel()
	delete(a.lives, chat)
	a.liveMu.Unlock()
	a.deleteLiveMessage(chat, msgID)
	return true
}

func (a *App) stopLive() {
	a.liveMu.Lock()
	runs := a.lives
	a.lives = nil
	a.liveMu.Unlock()
	for _, run := range runs {
		run.cancel()
	}
}

func (a *App) deleteLiveMessage(chat int64, messageID int) {
	if messageID == 0 {
		return
	}
	if err := a.tg.DeleteMessage(chat, messageID); err != nil {
		a.logf("live delete: %v", err)
	}
}

func parseLiveArgs(args []string, defaultStyle string) (stop bool, every time.Duration, style string, err error) {
	every = liveDefaultEvery
	style = defaultStyle
	if style != reportStyleShort && style != reportStyleFull {
		style = reportStyleFull
	}
	sawStyle := false
	sawEvery := false
	for _, raw := range args {
		low := strings.ToLower(strings.TrimSpace(raw))
		switch low {
		case "stop", "off", "стоп", "выкл":
			if len(args) != 1 {
				return false, 0, "", fmt.Errorf("stop")
			}
			return true, 0, "", nil
		}
		if parsed, ok := parseReportStyle(low); ok {
			if sawStyle {
				return false, 0, "", fmt.Errorf("style")
			}
			style = parsed
			sawStyle = true
			continue
		}
		d, ok := parseLiveEvery(low)
		if !ok {
			return false, 0, "", fmt.Errorf("interval")
		}
		if sawEvery {
			return false, 0, "", fmt.Errorf("interval")
		}
		every = d
		sawEvery = true
	}
	return false, every, style, nil
}

func parseLiveEvery(raw string) (time.Duration, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return clampLiveEvery(d)
	}
	raw = strings.TrimSuffix(raw, "сек")
	raw = strings.TrimSuffix(raw, "с")
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return clampLiveEvery(time.Duration(n) * time.Second)
}

func clampLiveEvery(d time.Duration) (time.Duration, bool) {
	if d < liveMinEvery || d > liveMaxEvery {
		return 0, false
	}
	return d, true
}

func trimTelegram(s string) string {
	r := []rune(s)
	if len(r) <= liveTextLimit {
		return s
	}
	return string(r[:liveTextLimit]) + "\n…"
}

func liveEditUnchanged(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "message is not modified")
}

func liveEditStopped(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "message to edit not found") ||
		strings.Contains(s, "message can't be edited") ||
		strings.Contains(s, "message_id_invalid")
}

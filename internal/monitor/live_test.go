package monitor

import (
	"strings"
	"testing"
	"time"
)

func TestParseLiveArgs(t *testing.T) {
	stop, every, style, err := parseLiveArgs(nil, reportStyleShort)
	if err != nil || stop || every != 30*time.Second || style != reportStyleShort {
		t.Fatalf("default: stop=%v every=%s style=%s err=%v", stop, every, style, err)
	}
	stop, _, _, err = parseLiveArgs([]string{"stop"}, reportStyleFull)
	if err != nil || !stop {
		t.Fatalf("stop: %v %v", stop, err)
	}
	_, every, style, err = parseLiveArgs([]string{"full", "45"}, reportStyleShort)
	if err != nil || every != 45*time.Second || style != reportStyleFull {
		t.Fatalf("override: every=%s style=%s err=%v", every, style, err)
	}
	_, every, style, err = parseLiveArgs([]string{"1m", "краткая"}, reportStyleFull)
	if err != nil || every != time.Minute || style != reportStyleShort {
		t.Fatalf("minute: every=%s style=%s err=%v", every, style, err)
	}
	if _, _, _, err = parseLiveArgs([]string{"3"}, reportStyleFull); err == nil {
		t.Fatal("expected the 10s minimum")
	}
	if _, _, _, err = parseLiveArgs([]string{"stop", "short"}, reportStyleFull); err == nil {
		t.Fatal("expected stop to be alone")
	}
}

func TestLiveCommandSendsEditableStatus(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	defer app.stopLive()
	app.replyTo = 99

	app.HandleCommand("/live short")
	if len(tg.sent) != 1 || len(tg.sentTo) != 1 || tg.sentTo[0] != 99 {
		t.Fatalf("sent %#v to %v", tg.sent, tg.sentTo)
	}
	if !strings.Contains(tg.sent[0], "Краткий статус") || strings.Contains(tg.sent[0], "15 мин") {
		t.Fatalf("short body: %s", tg.sent[0])
	}
	if !strings.Contains(tg.sent[0], "30s") || !strings.Contains(tg.sent[0], "/live stop") {
		t.Fatalf("footer: %s", tg.sent[0])
	}

	app.HandleCommand("/live full 15")
	last := tg.sent[len(tg.sent)-1]
	if !strings.Contains(last, "Полный статус") || !strings.Contains(last, "15 мин") || !strings.Contains(last, "15s") {
		t.Fatalf("full body: %s", last)
	}
	app.HandleCommand("/live stop")
	if tg.sent[len(tg.sent)-1] != "🛑 Живая сводка остановлена" {
		t.Fatalf("stop: %#v", tg.sent[len(tg.sent)-1])
	}
	if len(tg.deleted) != 2 {
		t.Fatalf("stop should delete the current message, deleted %v", tg.deleted)
	}
	app.HandleCommand("/live stop")
	if tg.sent[len(tg.sent)-1] != "Живая сводка не запущена" {
		t.Fatalf("second stop: %#v", tg.sent[len(tg.sent)-1])
	}
}

func TestLiveReplacesOnlySameChat(t *testing.T) {
	tg := &fakeTG{}
	app := newTestApp(t, tg)
	defer app.stopLive()

	app.replyTo = 99
	app.HandleCommand("/live short")
	app.HandleCommand("/live full")
	if len(tg.deleted) != 1 || tg.deleted[0] != [2]int64{99, 1} {
		t.Fatalf("same chat should drop the first message, deleted %v", tg.deleted)
	}

	app.replyTo = 50
	app.HandleCommand("/live")
	if len(tg.deleted) != 1 {
		t.Fatalf("another chat must keep its own live, deleted %v", tg.deleted)
	}
	if !app.liveCurrent(99, 2) || !app.liveCurrent(50, 3) {
		t.Fatal("each chat should keep its latest live message")
	}
}

func TestLiveEditHints(t *testing.T) {
	if !liveEditUnchanged(errStr("Bad Request: message is not modified")) {
		t.Fatal("unchanged edit should be ignored")
	}
	if !liveEditStopped(errStr("Bad Request: message to edit not found")) {
		t.Fatal("deleted message should stop the loop")
	}
	if liveEditStopped(errStr("timeout")) || liveEditUnchanged(errStr("timeout")) {
		t.Fatal("timeout should keep the loop")
	}
}

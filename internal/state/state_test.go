package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMuteAndCooldown(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	if s.IsMuted(now) {
		t.Fatal("should not be muted")
	}
	if err := s.Mute(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !s.IsMuted(now) {
		t.Fatal("should be muted")
	}
	if s.CanAlert("cpu", now, 15*time.Minute) {
		t.Fatal("muted alerts must be blocked")
	}
	if err := s.Unmute(); err != nil {
		t.Fatal(err)
	}
	if !s.CanAlert("cpu", now, 15*time.Minute) {
		t.Fatal("expected first alert")
	}
	if err := s.MarkAlert("cpu", now); err != nil {
		t.Fatal(err)
	}
	if s.CanAlert("cpu", now.Add(time.Minute), 15*time.Minute) {
		t.Fatal("cooldown should block")
	}
	if !s.CanAlert("cpu", now.Add(16*time.Minute), 15*time.Minute) {
		t.Fatal("cooldown expired")
	}
	if err := s.ClearAlert("cpu"); err != nil {
		t.Fatal(err)
	}
	if !s.CanAlert("cpu", now.Add(time.Minute), 15*time.Minute) {
		t.Fatal("cleared alert should fire")
	}
}

func TestReloadPersistsBoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	boot := time.Date(2026, 8, 25, 22, 57, 3, 0, time.UTC)
	if err := s.SetBoot("abc-123", boot); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Snapshot()
	if got.BootID != "abc-123" || !got.LastBootTime.Equal(boot) {
		t.Fatalf("got %+v", got)
	}
}

func TestClearAlertNoWriteWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClearAlert("cpu"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("should not create state file when clearing missing alert")
	}
}

func TestReportIntervalPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.ReportInterval() != 0 {
		t.Fatal("empty state should have no override")
	}
	if err := s.SetReportInterval(48 * time.Hour); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	if err := s.SetLastReportAt(at); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s2.ReportInterval() != 48*time.Hour {
		t.Fatalf("interval %s", s2.ReportInterval())
	}
	if !s2.LastReportAt().Equal(at) {
		t.Fatalf("last report %s", s2.LastReportAt())
	}
}

func TestReportStylePersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.ReportStyle() != "" {
		t.Fatal("empty state should have no style override")
	}
	if err := s.SetReportStyle("short"); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s2.ReportStyle() != "short" {
		t.Fatalf("style %q", s2.ReportStyle())
	}
}

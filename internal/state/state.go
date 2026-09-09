package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	mu    sync.Mutex
	path  string
	state File
}

type File struct {
	BootID           string               `json:"boot_id"`
	LastBootTime     time.Time            `json:"last_boot_time"`
	MutedUntil       time.Time            `json:"muted_until"`
	LastAlerts       map[string]time.Time `json:"last_alerts"`
	LastSeenAlive    time.Time            `json:"last_seen_alive"`
	ReportIntervalNs int64                `json:"report_interval_ns,omitempty"`
	LastReportAt     time.Time            `json:"last_report_at"`
	ReportStyle      string               `json:"report_style,omitempty"`
}

func Load(path string) (*Store, error) {
	s := &Store{path: path, state: File{LastAlerts: map[string]time.Time{}}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &s.state); err != nil {
		return nil, err
	}
	if s.state.LastAlerts == nil {
		s.state.LastAlerts = map[string]time.Time{}
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Snapshot() File {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := s.state
	cp.LastAlerts = map[string]time.Time{}
	for k, v := range s.state.LastAlerts {
		cp.LastAlerts[k] = v
	}
	return cp
}

func (s *Store) SetBoot(id string, bootTime time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.BootID = id
	s.state.LastBootTime = bootTime
	return s.saveLocked()
}

func (s *Store) TouchAlive() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.LastSeenAlive = time.Now().UTC()
	return s.saveLocked()
}

func (s *Store) Mute(until time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.MutedUntil = until
	return s.saveLocked()
}

func (s *Store) Unmute() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.MutedUntil = time.Time{}
	return s.saveLocked()
}

func (s *Store) IsMuted(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.MutedUntil.After(now)
}

func (s *Store) CanAlert(key string, now time.Time, cooldown time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.MutedUntil.After(now) {
		return false
	}
	last, ok := s.state.LastAlerts[key]
	if !ok {
		return true
	}
	return now.Sub(last) >= cooldown
}

func (s *Store) MarkAlert(key string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.LastAlerts == nil {
		s.state.LastAlerts = map[string]time.Time{}
	}
	s.state.LastAlerts[key] = now
	return s.saveLocked()
}

func (s *Store) ClearAlert(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.LastAlerts[key]; !ok {
		return nil
	}
	delete(s.state.LastAlerts, key)
	return s.saveLocked()
}

func (s *Store) ReportInterval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.ReportIntervalNs <= 0 {
		return 0
	}
	return time.Duration(s.state.ReportIntervalNs)
}

func (s *Store) SetReportInterval(d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.ReportIntervalNs = int64(d)
	return s.saveLocked()
}

func (s *Store) LastReportAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.LastReportAt
}

func (s *Store) SetLastReportAt(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.LastReportAt = t.UTC()
	return s.saveLocked()
}

func (s *Store) ReportStyle() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.ReportStyle
}

func (s *Store) SetReportStyle(style string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.ReportStyle = style
	return s.saveLocked()
}

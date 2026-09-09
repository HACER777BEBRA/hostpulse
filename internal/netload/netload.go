package netload

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"hostpulse/internal/metrics"

	gnet "github.com/shirou/gopsutil/v4/net"
)

type Sample struct {
	T  time.Time
	Rx uint64
	Tx uint64
}

type Sampler struct {
	mu     sync.Mutex
	iface  string
	auto   bool
	locked bool
	hist   []Sample
}

type Rates struct {
	Iface  string
	NowRx  float64
	NowTx  float64
	M1Rx   float64
	M1Tx   float64
	M15Rx  float64
	M15Tx  float64
	H1Rx   float64
	H1Tx   float64
	Warmup bool
}

func New(iface string) *Sampler {
	return &Sampler{iface: iface, auto: iface == "" || strings.EqualFold(iface, "auto")}
}

func (s *Sampler) Start(stop <-chan struct{}) {
	s.tick()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.tick()
		}
	}
}

func (s *Sampler) tick() {
	name, rx, tx, ok := s.readCounters()
	if !ok {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.iface != name {
		s.iface = name
		s.hist = s.hist[:0]
	}
	s.hist = append(s.hist, Sample{T: now, Rx: rx, Tx: tx})
	cutoff := now.Add(-70 * time.Minute)
	i := 0
	for i < len(s.hist) && s.hist[i].T.Before(cutoff) {
		i++
	}
	if i > 0 {
		s.hist = s.hist[i:]
	}
}

func (s *Sampler) Snapshot() Rates {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := Rates{Iface: s.iface, Warmup: len(s.hist) < 60}
	if len(s.hist) < 2 {
		return r
	}
	r.NowRx, r.NowTx = s.rateLocked(3 * time.Second)
	r.M1Rx, r.M1Tx = s.rateLocked(time.Minute)
	r.M15Rx, r.M15Tx = s.rateLocked(15 * time.Minute)
	r.H1Rx, r.H1Tx = s.rateLocked(time.Hour)
	return r
}

func (s *Sampler) rateLocked(window time.Duration) (rx, tx float64) {
	if len(s.hist) < 2 {
		return 0, 0
	}
	last := s.hist[len(s.hist)-1]
	from := last.T.Add(-window)
	first := s.hist[0]
	for _, sm := range s.hist {
		if !sm.T.Before(from) {
			first = sm
			break
		}
	}
	dt := last.T.Sub(first.T).Seconds()
	if dt <= 0 {
		return 0, 0
	}
	var drx, dtx float64
	if last.Rx >= first.Rx {
		drx = float64(last.Rx - first.Rx)
	}
	if last.Tx >= first.Tx {
		dtx = float64(last.Tx - first.Tx)
	}
	return drx / dt, dtx / dt
}

func (s *Sampler) readCounters() (name string, rx, tx uint64, ok bool) {
	stats, err := gnet.IOCounters(true)
	if err != nil || len(stats) == 0 {
		return "", 0, 0, false
	}

	s.mu.Lock()
	want := s.iface
	auto := s.auto && !s.locked
	s.mu.Unlock()

	if auto {
		want = detectDefaultIface(stats)
	}
	if want != "" {
		for _, st := range stats {
			if st.Name == want {
				s.lockIface(st.Name)
				return st.Name, st.BytesRecv, st.BytesSent, true
			}
		}
	}
	for _, st := range stats {
		if !skipIface(st.Name) {
			s.lockIface(st.Name)
			return st.Name, st.BytesRecv, st.BytesSent, true
		}
	}
	st := stats[0]
	s.lockIface(st.Name)
	return st.Name, st.BytesRecv, st.BytesSent, true
}

func (s *Sampler) lockIface(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.auto && !s.locked {
		s.iface = name
		s.locked = true
	}
}

func detectDefaultIface(stats []gnet.IOCountersStat) string {
	best := ""
	var bestN uint64
	for _, st := range stats {
		if skipIface(st.Name) {
			continue
		}
		n := st.BytesRecv + st.BytesSent
		if n >= bestN {
			bestN = n
			best = st.Name
		}
	}
	return best
}

func skipIface(name string) bool {
	n := strings.ToLower(name)
	if n == "lo" || n == "lo0" {
		return true
	}
	for _, p := range []string{"br-", "docker", "veth", "virbr", "amn", "tun", "tap", "awg", "wg"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

func humanRate(bps float64) string {
	if bps < 0 {
		bps = 0
	}
	mbit := bps * 8 / 1_000_000
	return fmt.Sprintf("%s/s (%.2f Mbit/s)", metrics.HumanBytes(uint64(bps)), mbit)
}

func (r Rates) Format() string {
	iface := r.Iface
	if iface == "" {
		iface = "?"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🌐 <b>Сеть</b> (<code>%s</code>)\n", iface)
	for _, row := range []struct {
		label string
		rx    float64
		tx    float64
	}{
		{"Сейчас", r.NowRx, r.NowTx},
		{"1 мин", r.M1Rx, r.M1Tx},
		{"15 мин", r.M15Rx, r.M15Tx},
		{"1 час", r.H1Rx, r.H1Tx},
	} {
		fmt.Fprintf(&b, "• %s: ↓ <b>%s</b>  ↑ <b>%s</b>\n", row.label, humanRate(row.rx), humanRate(row.tx))
	}
	out := strings.TrimRight(b.String(), "\n")
	if r.Warmup {
		out += "\n<i>Окна копятся после старта монитора</i>"
	}
	return out
}

func (r Rates) FormatCompact() string {
	iface := r.Iface
	if iface == "" {
		iface = "?"
	}
	return fmt.Sprintf("🌐 <code>%s</code>: ↓ <b>%s</b>  ↑ <b>%s</b>", iface, humanRate(r.NowRx), humanRate(r.NowTx))
}

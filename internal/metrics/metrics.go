package metrics

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

type Snapshot struct {
	Hostname    string
	CollectedAt time.Time
	Uptime      time.Duration
	BootTime    time.Time
	CPUPercent  float64
	CPUOK       bool
	Load1       float64
	Load5       float64
	Load15      float64
	LoadOK      bool
	MemPercent  float64
	MemUsed     uint64
	MemTotal    uint64
	MemOK       bool
	SwapPercent float64
	SwapUsed    uint64
	SwapTotal   uint64
	DiskPercent float64
	DiskUsed    uint64
	DiskFree    uint64
	DiskTotal   uint64
	DiskOK      bool
	TopCPU      []Proc
	TopRAM      []Proc
}

type Proc struct {
	PID  int32
	Name string
	CPU  float64
	RSS  uint64
}

func Collect(hostname, diskPath string, withProcs bool) Snapshot {
	s := Snapshot{Hostname: hostname, CollectedAt: time.Now().UTC()}
	if s.Hostname == "" {
		if h, err := os.Hostname(); err == nil {
			s.Hostname = h
		}
	}

	if percents, err := cpu.Percent(time.Second, false); err == nil && len(percents) > 0 {
		s.CPUPercent = percents[0]
		s.CPUOK = true
	}
	if avg, err := load.Avg(); err == nil {
		s.Load1, s.Load5, s.Load15 = avg.Load1, avg.Load5, avg.Load15
		s.LoadOK = true
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemPercent = vm.UsedPercent
		s.MemUsed = vm.Used
		s.MemTotal = vm.Total
		s.MemOK = true
	}
	if sm, err := mem.SwapMemory(); err == nil {
		s.SwapPercent = sm.UsedPercent
		s.SwapUsed = sm.Used
		s.SwapTotal = sm.Total
	}
	if du, err := disk.Usage(diskPath); err == nil {
		s.DiskPercent = du.UsedPercent
		s.DiskUsed = du.Used
		s.DiskFree = du.Free
		s.DiskTotal = du.Total
		s.DiskOK = true
	}
	if bt, err := host.BootTime(); err == nil {
		s.BootTime = time.Unix(int64(bt), 0).UTC()
	}
	if up, err := host.Uptime(); err == nil {
		s.Uptime = time.Duration(up) * time.Second
	}
	if withProcs {
		s.TopCPU, s.TopRAM = topProcesses(8)
	}
	return s
}

func topProcesses(n int) (cpuTop, ramTop []Proc) {
	list, err := process.Processes()
	if err != nil {
		return nil, nil
	}
	type item struct {
		p    *process.Process
		proc Proc
	}
	items := make([]item, 0, len(list))
	for _, p := range list {
		name, _ := p.Name()
		var rss uint64
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			rss = mi.RSS
		}
		items = append(items, item{p: p, proc: Proc{PID: p.Pid, Name: name, RSS: rss}})
		_, _ = p.CPUPercent()
	}
	time.Sleep(400 * time.Millisecond)

	out := make([]Proc, 0, len(items))
	for _, it := range items {
		pct, _ := it.p.CPUPercent()
		it.proc.CPU = pct
		if it.proc.Name == "" && it.proc.RSS == 0 && it.proc.CPU == 0 {
			continue
		}
		out = append(out, it.proc)
	}

	cpuTop = append([]Proc(nil), out...)
	sort.Slice(cpuTop, func(i, j int) bool { return cpuTop[i].CPU > cpuTop[j].CPU })
	ramTop = append([]Proc(nil), out...)
	sort.Slice(ramTop, func(i, j int) bool { return ramTop[i].RSS > ramTop[j].RSS })
	if len(cpuTop) > n {
		cpuTop = cpuTop[:n]
	}
	if len(ramTop) > n {
		ramTop = ramTop[:n]
	}
	return cpuTop, ramTop
}

func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Second)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	return fmt.Sprintf("%dh %dm %ds", hours, mins, secs)
}

func HumanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for n/div >= unit && exp < 4 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), []string{"KiB", "MiB", "GiB", "TiB", "PiB"}[exp])
}

func Escape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func (s Snapshot) FormatStatus() string {
	var b strings.Builder
	fmt.Fprintf(&b, "🖥 <b>%s</b>\n", Escape(s.Hostname))
	fmt.Fprintf(&b, "⏱ Uptime: <code>%s</code>\n", FormatDuration(s.Uptime))
	fmt.Fprintf(&b, "📅 Boot: <code>%s</code>\n\n", s.BootTime.UTC().Format("2006-01-02 15:04 UTC"))
	fmt.Fprintf(&b, "⚙️ CPU: <b>%.1f%%</b>\n", s.CPUPercent)
	fmt.Fprintf(&b, "📈 Load: <code>%.2f %.2f %.2f</code>\n", s.Load1, s.Load5, s.Load15)
	fmt.Fprintf(&b, "🧠 RAM: <b>%.1f%%</b> (%s / %s)\n", s.MemPercent, HumanBytes(s.MemUsed), HumanBytes(s.MemTotal))
	if s.SwapTotal == 0 {
		b.WriteString("💾 Swap: <code>нет</code>\n")
	} else {
		fmt.Fprintf(&b, "💾 Swap: <b>%.1f%%</b> (%s / %s)\n", s.SwapPercent, HumanBytes(s.SwapUsed), HumanBytes(s.SwapTotal))
	}
	fmt.Fprintf(&b, "💽 Disk: <b>%.1f%%</b> (free %s / %s)", s.DiskPercent, HumanBytes(s.DiskFree), HumanBytes(s.DiskTotal))
	return b.String()
}

func (s Snapshot) FormatStatusCompact() string {
	var b strings.Builder
	fmt.Fprintf(&b, "🖥 <b>%s</b>\n", Escape(s.Hostname))
	fmt.Fprintf(&b, "⚙️ CPU <b>%.1f%%</b> · 🧠 RAM <b>%.1f%%</b> · 💽 Disk <b>%.1f%%</b> · 📈 Load <code>%.2f</code>\n",
		s.CPUPercent, s.MemPercent, s.DiskPercent, s.Load1)
	fmt.Fprintf(&b, "⏱ Uptime <code>%s</code>", FormatDuration(s.Uptime))
	return b.String()
}

func (s Snapshot) FormatProcesses() string {
	var b strings.Builder
	fmt.Fprintf(&b, "⏱ Uptime: <code>%s</code>\n", FormatDuration(s.Uptime))
	fmt.Fprintf(&b, "📅 Boot: <code>%s</code>\n\n", s.BootTime.UTC().Format("2006-01-02 15:04 UTC"))
	b.WriteString("🔥 <b>Топ по CPU</b>\n")
	if len(s.TopCPU) == 0 {
		b.WriteString("нет данных\n")
	} else {
		for i, p := range s.TopCPU {
			fmt.Fprintf(&b, "%d. <code>%s</code> pid=%d — %.1f%% / %s\n", i+1, Escape(p.Name), p.PID, p.CPU, HumanBytes(p.RSS))
		}
	}
	b.WriteString("\n🧠 <b>Топ по RAM</b>\n")
	if len(s.TopRAM) == 0 {
		b.WriteString("нет данных")
	} else {
		for i, p := range s.TopRAM {
			fmt.Fprintf(&b, "%d. <code>%s</code> pid=%d — %s / CPU %.1f%%\n", i+1, Escape(p.Name), p.PID, HumanBytes(p.RSS), p.CPU)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

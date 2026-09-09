package reboot

import (
	"os"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/host"
)

type Info struct {
	BootID   string
	BootTime time.Time
	Uptime   time.Duration
}

func Detect() (Info, error) {
	id, err := readBootID()
	if err != nil {
		return Info{}, err
	}
	bt, err := host.BootTime()
	if err != nil {
		return Info{}, err
	}
	up, err := host.Uptime()
	if err != nil {
		return Info{}, err
	}
	return Info{
		BootID:   strings.TrimSpace(id),
		BootTime: time.Unix(int64(bt), 0).UTC(),
		Uptime:   time.Duration(up) * time.Second,
	}, nil
}

func readBootID() (string, error) {
	data, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func Short(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 8 {
		return id
	}
	return id[:8] + "…"
}

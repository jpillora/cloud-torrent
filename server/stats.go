package server

import (
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// Stats is the version/runtime/resource panel rendered in the footer.
type Stats struct {
	Title   string
	Version string
	Runtime string
	Uptime  time.Time
	System  SystemStats
}

// SystemStats is a snapshot of host and Go-process resource usage.
type SystemStats struct {
	Set         bool    `json:"set"`
	CPU         float64 `json:"cpu"`
	DiskUsed    int64   `json:"diskUsed"`
	DiskTotal   int64   `json:"diskTotal"`
	MemoryUsed  int64   `json:"memoryUsed"`
	MemoryTotal int64   `json:"memoryTotal"`
	GoMemory    int64   `json:"goMemory"`
	GoRoutines  int     `json:"goRoutines"`
}

// load refreshes the snapshot; diskDir selects the filesystem to report on.
func (s *SystemStats) load(diskDir string) {
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) == 1 {
		s.CPU = pcts[0]
	}
	if u, err := disk.Usage(diskDir); err == nil {
		s.DiskUsed = int64(u.Used)
		s.DiskTotal = int64(u.Total)
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemoryUsed = int64(vm.Used)
		s.MemoryTotal = int64(vm.Total)
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.GoMemory = int64(m.Alloc)
	s.GoRoutines = runtime.NumGoroutine()
	s.Set = true
}

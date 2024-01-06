package server

import (
	"runtime"

	velox "github.com/jpillora/velox/go"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

type stats struct {
	Set         bool    `json:"set"`
	CPU         float64 `json:"cpu"`
	DiskUsed    int64   `json:"diskUsed"`
	DiskTotal   int64   `json:"diskTotal"`
	MemoryUsed  int64   `json:"memoryUsed"`
	MemoryTotal int64   `json:"memoryTotal"`
	GoMemory    int64   `json:"goMemory"`
	GoRoutines  int     `json:"goRoutines"`
	//internal
	pusher velox.Pusher
}

func (s *stats) loadStats(diskDir string) {
	//count cpu cycles between last count
	if percents, err := cpu.Percent(0, false); err == nil && len(percents) == 1 {
		s.CPU = percents[0]
	}
	//count disk usage
	if stat, err := disk.Usage(diskDir); err == nil {
		s.DiskUsed = int64(stat.Used)
		s.DiskTotal = int64(stat.Total)
	}
	//count memory usage
	if stat, err := mem.VirtualMemory(); err == nil {
		s.MemoryUsed = int64(stat.Used)
		s.MemoryTotal = int64(stat.Total)
	}
	//count total bytes allocated by the go runtime
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	s.GoMemory = int64(memStats.Alloc)
	//count current number of goroutines
	s.GoRoutines = runtime.NumGoroutine()
	//done
	s.Set = true
	s.pusher.Push()
}

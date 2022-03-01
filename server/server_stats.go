package server

import (
	"runtime"

	velox "github.com/jpillora/velox/go"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
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
	lastCPUStat *cpu.TimesStat
	pusher      velox.Pusher
}

func (s *stats) loadStats(diskDir string) {
	//count cpu cycles between last count
	if stats, err := cpu.Times(false); err == nil {
		stat := stats[0]
		total := totalCPUTime(stat)
		last := s.lastCPUStat
		if last != nil {
			lastTotal := totalCPUTime(*last)
			if lastTotal != 0 {
				totalDelta := total - lastTotal
				if totalDelta > 0 {
					idleDelta := (stat.Iowait + stat.Idle) - (last.Iowait + last.Idle)
					usedDelta := (totalDelta - idleDelta)
					s.CPU = 100 * usedDelta / totalDelta
				}
			}
		}
		s.lastCPUStat = &stat
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

func totalCPUTime(t cpu.TimesStat) float64 {
	total := t.User + t.System + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest + t.GuestNice + t.Idle
	return total
}

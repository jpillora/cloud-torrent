package server

import (
	"os"
	"runtime"

	velox "github.com/jpillora/velox/go"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
)

type stats struct {
	Set             bool    `json:"set"`
	CPU             float64 `json:"cpu"`
	DiskFree        uint64  `json:"diskFree"`
	DiskUsedPercent float64 `json:"diskUsedPercent"`
	MemUsedPercent  float64 `json:"memUsedPercent"`
	GoMemory        int64   `json:"goMemory"`
	GoRoutines      int     `json:"goRoutines"`
	//internal
	pusher velox.Pusher
}

func (s *stats) Push() {
	s.pusher.Push()
}

func (s *stats) loadStats(diskDir string) {
	//count cpu cycles between last count
	//count disk usage
	if cpu, err := cpu.Percent(0, false); err == nil {
		s.CPU = cpu[0]
	}
	if stat, err := disk.Usage(diskDir); err == nil {
		s.DiskUsedPercent = stat.UsedPercent
		s.DiskFree = stat.Free
	}
	//count memory usage
	if stat, err := mem.VirtualMemory(); err == nil {
		s.MemUsedPercent = stat.UsedPercent
	}
	//count total bytes allocated by the go runtime
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	s.GoMemory = int64(memStats.Alloc)
	//count current number of goroutines
	s.GoRoutines = runtime.NumGoroutine()
	//done
	s.Set = true
}

func detectDiskStat(dir string) error {

	if err := os.Mkdir(dir, os.ModePerm); err != nil {
		if !os.IsExist(err) {
			return err
		}
	}

	stat, err := disk.Usage(dir)
	if err != nil {
		return err
	}

	if stat.Free < 10*1024*1024 {
		return ErrDiskSpace
	}

	return nil
}

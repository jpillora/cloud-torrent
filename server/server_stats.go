package server

import (
	"os"
	"runtime"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

type osStats struct {
	CPU             float64 `json:"cpu"`
	DiskFree        uint64  `json:"diskFree"`
	DiskUsedPercent float64 `json:"diskUsedPercent"`
	MemUsedPercent  float64 `json:"memUsedPercent"`
	GoMemory        int64   `json:"goMemory"`
	GoRoutines      int     `json:"goRoutines"`
	//internal
	diskDirPath string
}

func (s *osStats) loadStats() {
	//count cpu cycles between last count
	//count disk usage
	if cpu, err := cpu.Percent(0, false); err == nil {
		s.CPU = cpu[0]
	}
	if stat, err := disk.Usage(s.diskDirPath); err == nil {
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
	s.GoMemory = int64(memStats.Sys)
	//count current number of goroutines
	s.GoRoutines = runtime.NumGoroutine()
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

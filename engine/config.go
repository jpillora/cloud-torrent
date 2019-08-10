package engine

import "golang.org/x/time/rate"

type Config struct {
	AutoStart            bool
	Debug                bool
	ObfsPreferred        bool
	ObfsRequirePreferred bool
	DisableTrackers      bool
	DisableIPv6          bool
	DownloadDirectory    string
	WatchDirectory       string
	EnableUpload         bool
	EnableSeeding        bool
	IncomingPort         int
	DoneCmd              string
	SeedRatio            float32
	UploadRate           string
	DownloadRate         string
	TrackerListURL       string
}

func (c *Config) UploadLimiter() *rate.Limiter {
	return rateLimiter(c.UploadRate)
}

func (c *Config) DownloadLimiter() *rate.Limiter {
	return rateLimiter(c.DownloadRate)
}

func rateLimiter(rstr string) *rate.Limiter {
	var rateSize int
	switch rstr {
	case "Low":
		rateSize = 50000
	case "Medium":
		rateSize = 500000
	case "High":
		rateSize = 1500000
	default:
		return rate.NewLimiter(rate.Inf, 0)
	}
	return rate.NewLimiter(rate.Limit(rateSize), rateSize)
}

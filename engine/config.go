package engine

import (
	"log"
	"strings"

	"github.com/c2h5oh/datasize"
	"golang.org/x/time/rate"
)

type Config struct {
	AutoStart            bool
	EngineDebug          bool
	MuteEngineLog        bool
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
	AlwaysAddTrackers    bool
	ProxyURL             string
}

func (c *Config) UploadLimiter() *rate.Limiter {
	return rateLimiter(c.UploadRate)
}

func (c *Config) DownloadLimiter() *rate.Limiter {
	return rateLimiter(c.DownloadRate)
}

func rateLimiter(rstr string) *rate.Limiter {
	var rateSize int
	rstr = strings.TrimSpace(rstr)
	switch rstr {
	case "Low", "low":
		// ~50k/s
		rateSize = 50000
	case "Medium", "medium":
		// ~500k/s
		rateSize = 500000
	case "High", "high":
		// ~1500k/s
		rateSize = 1500000
	case "Unlimited", "unlimited", "0", "":
		// unlimited
		return rate.NewLimiter(rate.Inf, 0)
	default:
		var v datasize.ByteSize
		err := v.UnmarshalText([]byte(rstr))
		if err != nil {
			log.Printf("RateLimit [%s] unreconized, set as unlimited", rstr)
			return rate.NewLimiter(rate.Inf, 0)
		}
		if v > 2147483647 {
			// max of int, unlimited
			return rate.NewLimiter(rate.Inf, 0)
		}

		rateSize = int(v)
	}
	return rate.NewLimiter(rate.Limit(rateSize), rateSize)
}

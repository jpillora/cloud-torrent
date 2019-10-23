package engine

import (
	"log"
	"reflect"
	"strings"

	"github.com/c2h5oh/datasize"
	"golang.org/x/time/rate"
)

const (
	ForbidRuntimeChange uint8 = 1 << iota
	NeedEngineReConfig
	NeedRestartWatch
	NeedUpdateTracker
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
	RssURL               string
}

func (c *Config) UploadLimiter() *rate.Limiter {
	return rateLimiter(c.UploadRate)
}

func (c *Config) DownloadLimiter() *rate.Limiter {
	return rateLimiter(c.DownloadRate)
}

func (c *Config) Validate(nc *Config) uint8 {

	var status uint8

	if c.DoneCmd != nc.DoneCmd {
		status |= ForbidRuntimeChange
	}
	if c.WatchDirectory != nc.WatchDirectory {
		status |= NeedRestartWatch
	}
	if c.TrackerListURL != nc.TrackerListURL {
		status |= NeedUpdateTracker
	}

	rfc := reflect.ValueOf(c)
	rfnc := reflect.ValueOf(nc)

	for _, field := range []string{"IncomingPort", "DownloadDirectory",
		"EngineDebug", "EnableUpload", "EnableSeeding", "UploadRate",
		"DownloadRate", "ObfsPreferred", "ObfsRequirePreferred",
		"DisableTrackers", "DisableIPv6", "ProxyURL"} {

		cval := reflect.Indirect(rfc).FieldByName(field)
		ncval := reflect.Indirect(rfnc).FieldByName(field)

		if cval.Interface() != ncval.Interface() {
			status |= NeedEngineReConfig
			break
		}
	}

	return status
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

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
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
	l, err := rateLimiter(c.UploadRate)
	if err != nil {
		c.UploadRate = ""
		log.Printf("RateLimit [%s] unreconized, set as unlimited", c.UploadRate)
		return rate.NewLimiter(rate.Inf, 0)
	}
	return l
}

func (c *Config) DownloadLimiter() *rate.Limiter {
	l, err := rateLimiter(c.DownloadRate)
	if err != nil {
		c.DownloadRate = ""
		log.Printf("RateLimit [%s] unreconized, set as unlimited", c.DownloadRate)
		return rate.NewLimiter(rate.Inf, 0)
	}
	return l
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

func (c *Config) SaveConfigFile(configPath string) error {
	b, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return err
	}

	ob, err := ioutil.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		if bytes.Compare(b, ob) == 0 {
			return nil
		}
	}

	return ioutil.WriteFile(configPath, b, 0644)
}

func rateLimiter(rstr string) (*rate.Limiter, error) {
	var rateSize int
	rstr = strings.ToLower(strings.TrimSpace(rstr))
	switch rstr {
	case "low":
		// ~50k/s
		rateSize = 50000
	case "medium":
		// ~500k/s
		rateSize = 500000
	case "high":
		// ~1500k/s
		rateSize = 1500000
	case "unlimited", "0", "":
		// unlimited
		return rate.NewLimiter(rate.Inf, 0), nil
	default:
		var v datasize.ByteSize
		err := v.UnmarshalText([]byte(rstr))
		if err != nil {
			return nil, err
		}
		if v > 2147483647 {
			// max of int, unlimited
			return nil, errors.New("excceed int val")
		}

		rateSize = int(v)
	}
	return rate.NewLimiter(rate.Limit(rateSize), rateSize*3), nil
}

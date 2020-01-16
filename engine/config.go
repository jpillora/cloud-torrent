package engine

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/c2h5oh/datasize"
	"github.com/spf13/viper"
	"golang.org/x/time/rate"
)

const (
	ForbidRuntimeChange uint8 = 1 << iota
	NeedEngineReConfig
	NeedRestartWatch
	NeedUpdateTracker
)

const (
	defaultTrackerListURL = "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_best.txt"
	defaultScraperURL     = "https://raw.githubusercontent.com/boypt/simple-torrent/master/scraper-config.json"
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
	DoneCmdThreshold     string
	SeedRatio            float32
	UploadRate           string
	DownloadRate         string
	TrackerListURL       string
	AlwaysAddTrackers    bool
	ProxyURL             string
	RssURL               string
	ScraperURL           string
}

func InitConf(specPath string) (*Config, error) {

	viper.SetConfigName("cloud-torrent")
	viper.AddConfigPath("/etc/cloud-torrent/")
	viper.AddConfigPath("/etc/")
	viper.AddConfigPath("$HOME/.cloud-torrent")
	viper.AddConfigPath(".")

	viper.SetDefault("DownloadDirectory", "./downloads")
	viper.SetDefault("WatchDirectory", "./torrents")
	viper.SetDefault("EnableUpload", true)
	viper.SetDefault("AutoStart", true)
	viper.SetDefault("DoneCmd", "")
	viper.SetDefault("DoneCmdThreshold", "30s")
	viper.SetDefault("SeedRatio", 0)
	viper.SetDefault("ObfsPreferred", true)
	viper.SetDefault("ObfsRequirePreferred", false)
	viper.SetDefault("IncomingPort", 50007)
	viper.SetDefault("TrackerListURL", defaultTrackerListURL)
	viper.SetDefault("ScraperURL", defaultScraperURL)

	// user specific config path
	if stat, err := os.Stat(specPath); stat != nil && err == nil {
		viper.SetConfigFile(specPath)
	}

	configExists := true
	if err := viper.ReadInConfig(); err != nil {
		if strings.Contains(err.Error(), "Not Found") {
			log.Println("[viper Config]", err)
			configExists = false
			if specPath == "" {
				specPath = "./cloud-torrent.yaml"
			}
			viper.SetConfigFile(specPath)
		} else {
			return nil, err
		}
	}

	c := &Config{}
	viper.Unmarshal(c)

	dirChanged, err := c.NormlizeConfigDir()
	if err != nil {
		return nil, err
	}
	if dirChanged {
		viper.Set("DownloadDirectory", c.DownloadDirectory)
		viper.Set("WatchDirectory", c.WatchDirectory)
	}

	cf := viper.ConfigFileUsed()
	log.Println("[config] selected config file: ", cf)
	if !configExists || dirChanged {
		if err := viper.WriteConfig(); err != nil {
			return nil, err
		}
		log.Println("[config] config file written: ", cf, "exists:", configExists, "dirchanged", dirChanged)
	}

	return c, nil
}

func (c *Config) NormlizeConfigDir() (bool, error) {
	var changed bool
	if c.DownloadDirectory != "" {
		dldir, err := filepath.Abs(c.DownloadDirectory)
		if err != nil {
			return false, fmt.Errorf("Invalid path %s, %w", c.WatchDirectory, err)
		}
		if c.DownloadDirectory != dldir {
			changed = true
			c.DownloadDirectory = dldir
		}
	}

	if c.WatchDirectory != "" {
		wdir, err := filepath.Abs(c.WatchDirectory)
		if err != nil {
			return false, fmt.Errorf("Invalid path %s, %w", c.WatchDirectory, err)
		}
		if c.WatchDirectory != wdir {
			changed = true
			c.WatchDirectory = wdir
		}
	}

	return changed, nil
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

func (c *Config) SyncViper(nc Config) error {
	cv := reflect.ValueOf(*c)
	nv := reflect.ValueOf(nc)
	typeOfC := cv.Type()
	for i := 0; i < typeOfC.NumField(); i++ {
		if cv.Field(i).Interface() != nv.Field(i).Interface() {
			name := typeOfC.Field(i).Name
			oval := cv.Field(i).Interface()
			val := nv.Field(i).Interface()
			viper.Set(name, val)
			log.Println("config updated ", name, ": ", oval, " -> ", val)
		}
	}

	return viper.WriteConfig()
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

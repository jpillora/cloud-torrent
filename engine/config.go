package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/boypt/simple-torrent/common"
	"github.com/spf13/viper"
	"golang.org/x/time/rate"
	"gopkg.in/yaml.v2"
)

const (
	ForbidRuntimeChange uint8 = 1 << iota
	NeedEngineReConfig
	NeedRestartWatch
	NeedUpdateTracker
	NeedLoadWaitList
	NeedUpdateRSS
)

const (
	defaultTrackerListURL = "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_best.txt"
	defaultConfigFile     = "cloud-torrent"
)

type Config struct {
	AutoStart               bool          `yaml:"AutoStart"`
	EngineDebug             bool          `yaml:"EngineDebug"`
	MuteEngineLog           bool          `yaml:"MuteEngineLog"`
	ObfsPreferred           bool          `yaml:"ObfsPreferred"`
	ObfsRequirePreferred    bool          `yaml:"ObfsRequirePreferred"`
	DisableTrackers         bool          `yaml:"DisableTrackers"`
	DisableIPv6             bool          `yaml:"DisableIPv6"`
	NoDefaultPortForwarding bool          `yaml:"NoDefaultPortForwarding"`
	DisableUTP              bool          `yaml:"DisableUTP"`
	DownloadDirectory       string        `yaml:"DownloadDirectory"`
	WatchDirectory          string        `yaml:"WatchDirectory"`
	EnableUpload            bool          `yaml:"EnableUpload"`
	EnableSeeding           bool          `yaml:"EnableSeeding"`
	IncomingPort            int           `yaml:"IncomingPort"`
	DoneCmd                 string        `yaml:"DoneCmd"`
	SeedRatio               float32       `yaml:"SeedRatio"`
	SeedTime                time.Duration `yaml:"SeedTime"`
	UploadRate              string        `yaml:"UploadRate"`
	DownloadRate            string        `yaml:"DownloadRate"`
	TrackerList             string        `yaml:"TrackerList"`
	AlwaysAddTrackers       bool          `yaml:"AlwaysAddTrackers"`
	ProxyURL                string        `yaml:"ProxyURL"`
	RssURL                  string        `yaml:"RssURL"`
	ScraperURL              string        `yaml:"ScraperURL"`
	MaxConcurrentTask       int           `yaml:"MaxConcurrentTask"`
	AllowRuntimeConfigure   bool          `yaml:"AllowRuntimeConfigure"`
}

func InitConf(specPath *string) (*Config, error) {
	if *specPath != "" {
		// user specific config path
		viper.SetConfigFile(*specPath)
	} else {
		viper.SetConfigName(defaultConfigFile)
		viper.AddConfigPath("/etc/")
		viper.AddConfigPath(".")
	}

	viper.SetDefault("DownloadDirectory", "./downloads")
	viper.SetDefault("WatchDirectory", "./torrents")
	viper.SetDefault("EnableUpload", true)
	viper.SetDefault("EnableSeeding", true)
	viper.SetDefault("NoDefaultPortForwarding", true)
	viper.SetDefault("DisableUTP", false)
	viper.SetDefault("AutoStart", true)
	viper.SetDefault("DoneCmd", "")
	viper.SetDefault("SeedRatio", 0)
	viper.SetDefault("SeedTime", "0")
	viper.SetDefault("ObfsPreferred", true)
	viper.SetDefault("ObfsRequirePreferred", false)
	viper.SetDefault("IncomingPort", 50007)
	viper.SetDefault("MaxConcurrentTask", 0)
	viper.SetDefault("AllowRuntimeConfigure", true)

	configExists := true
	if err := viper.ReadInConfig(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// user set a config that is not exists, will write to it later
			configExists = false
		} else {
			// write a default config file if not exists and not provided
			c := &Config{}
			common.HandleError(viper.Unmarshal(c))
			cn := defaultConfigFile + ".yaml"
			common.HandleError(c.WriteYaml(cn))
			viper.SetConfigFile(cn)
			log.Println("saved default config", cn)
		}
	}

	*specPath = viper.ConfigFileUsed()
	log.Println("[config] using config file: ", *specPath)

	c := &Config{}
	common.HandleError(viper.Unmarshal(c))

	dirChanged, err := c.NormlizeConfigDir()
	if err != nil {
		return nil, err
	}
	if dirChanged {
		viper.Set("DownloadDirectory", c.DownloadDirectory)
		viper.Set("WatchDirectory", c.WatchDirectory)
	}

	if !configExists || dirChanged {
		if err := c.WriteDefault(); err != nil {
			return nil, err
		}
		log.Println("[config] config file updated: ", *specPath, "exists:", configExists, "dirchanged", dirChanged)
	}

	return c, nil
}

func (c *Config) NormlizeConfigDir() (bool, error) {
	var changed bool
	if c.DownloadDirectory != "" {
		dldir, err := filepath.Abs(c.DownloadDirectory)
		if err != nil {
			return false, fmt.Errorf("ERROR: Invalid path %s, %w", c.WatchDirectory, err)
		}
		if c.DownloadDirectory != dldir {
			changed = true
			c.DownloadDirectory = dldir
		}
	}

	if c.WatchDirectory != "" {
		wdir, err := filepath.Abs(c.WatchDirectory)
		if err != nil {
			return false, fmt.Errorf("ERROR: Invalid path %s, %w", c.WatchDirectory, err)
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
	if c.TrackerList != nc.TrackerList {
		status |= NeedUpdateTracker
	}
	if c.MaxConcurrentTask < nc.MaxConcurrentTask {
		status |= NeedLoadWaitList
	}
	if c.RssURL != nc.RssURL {
		status |= NeedUpdateRSS
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

func (c *Config) SyncViper(nc Config) {
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
}

func (c *Config) WriteDefault() error {
	cf := viper.ConfigFileUsed()
	cfext := strings.ToLower(filepath.Ext(cf))
	if cfext == "yml" || cfext == "yaml" {
		// keeps keys cases
		return c.WriteYaml(cf)
	}
	// viper's write make all keys lowercased
	return viper.WriteConfig()
}

func (c *Config) WriteYaml(cf string) error {
	d, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(cf, d, 0666)
}

func (c *Config) GetCmdConfig() (string, []string, error) {
	if c.DoneCmd == "" {
		return "", nil, fmt.Errorf("unconfigred Donecmd")
	}
	env := append(os.Environ(), fmt.Sprintf("CLD_DIR=%s", c.DownloadDirectory))
	return c.DoneCmd, env, nil
}

package server

import (
	"compress/gzip"
	"fmt"
	stdlog "log"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/boypt/simple-torrent/common"
	"github.com/boypt/simple-torrent/server/httpmiddleware"

	"errors"

	"github.com/NYTimes/gziphandler"
	"github.com/anacrolix/torrent"
	"github.com/boypt/scraper"
	"github.com/boypt/simple-torrent/engine"
	ctstatic "github.com/boypt/simple-torrent/static"
	"github.com/jpillora/cookieauth"
	"github.com/jpillora/requestlog"
	"github.com/jpillora/velox"
	"github.com/mmcdole/gofeed"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/viper"
)

const (
	scraperUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/57.0.2987.133 Safari/537.36"
)

var (
	isListenOnUnix bool
	log            *stdlog.Logger
	//ErrDiskSpace raised if disk space not enough
	ErrDiskSpace = errors.New("not enough disk space")
)

//Server is the "State" portion of the diagram
type Server struct {
	//config
	Title          string `opts:"help=Title of this instance,env=TITLE"`
	Port           int    `opts:"help=Depreciated. use --listen. Listening port(),env=PORT"`
	Host           string `opts:"help=Depreciated. use --listen. Listening interface,env=HOST"`
	Listen         string `opts:"help=Listening Address:Port or unix socket (default all),env=LISTEN"`
	UnixPerm       string `opts:"help=DomainSocket file permission (default 0666),env=UNIXPERM"`
	Auth           string `opts:"help=Optional basic auth in form 'user:password',env=AUTH"`
	ProxyURL       string `opts:"help=Proxy url,env=PROXY_URL"`
	ConfigPath     string `opts:"help=Configuration file path (default ./cloud-torrent.yaml),short=c,env=CONFIGPATH"`
	KeyPath        string `opts:"help=TLS Key file path"`
	CertPath       string `opts:"help=TLS Certicate file path,short=r"`
	RestAPI        string `opts:"help=Listen on a trusted port accepts /api/ requests (eg. localhost:3001),env=RESTAPI"`
	ReqLog         bool   `opts:"help=Enable request logging,env=REQLOG"`
	Open           bool   `opts:"help=Open now with your default browser"`
	DisableLogTime bool   `opts:"help=Don't print timestamp in log,env=DISABLELOGTIME"`
	DisableMmap    bool   `opts:"help=Don't use mmap,env=DISABLEMMAP"`
	Debug          bool   `opts:"help=Debug app,env=DEBUG"`
	DebugTorrent   bool   `opts:"help=Debug torrent engine,env=DEBUGTORRENT"`
	ConvYAML       bool   `opts:"help=Convert old json config to yaml format."`
	IntevalSec     int    `opts:"help=Inteval seconds to push data to clients (default 3),env=INTEVALSEC"`

	//http handlers
	scraperh, dlfilesh, statich, verStatich, rssh http.Handler
	scraper                                       *scraper.Handler

	//torrent engine
	engine *engine.Engine

	//sync req
	syncConnected chan struct{}
	syncWg        sync.WaitGroup
	syncSemphor   int32

	state struct {
		velox.State
		UseQueue      bool
		LatestRSSGuid string
		Torrents      *map[string]*engine.Torrent
		Users         map[string]struct{}
		Stats         struct {
			System   osStats
			ConnStat torrent.ConnStats
		}
	}

	rssMark         map[string]string
	rssCache        []*gofeed.Item
	searchProviders *scraper.Config
	engineConfig    *engine.Config
	tpl             *TPLInfo
}

// Run the server
func (s *Server) Run(tpl *TPLInfo) error {

	s.tpl = tpl

	if s.IntevalSec <= 0 {
		s.IntevalSec = 3
	}

	if s.DisableLogTime {
		engine.SetLoggerFlag(stdlog.Lmsgprefix)
		log.SetFlags(stdlog.Lmsgprefix)
	}

	if s.Host != "" || s.Port != 3000 {
		log.Println("WARNING: --host --port arguments are depreciated, use --linsten instead, eg:`--listen :3000`")
		s.Listen = fmt.Sprintf("%s:%d", s.Host, s.Port)
		if strings.HasPrefix(s.Host, "unix:") {
			s.Listen = s.Host
		}
	}
	isListenOnUnix = strings.HasPrefix(s.Listen, "unix:")

	isTLS := s.CertPath != "" || s.KeyPath != "" //poor man's XOR
	if isTLS && (s.CertPath == "" || s.KeyPath == "") {
		return fmt.Errorf("ERROR: You must provide both key and cert paths")
	}

	s.syncConnected = make(chan struct{})
	//init maps
	s.state.Users = make(map[string]struct{})
	s.rssMark = make(map[string]string)

	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.statich = ctstatic.FileSystemHandler()
	s.verStatich = http.StripPrefix("/"+s.tpl.Version, s.statich)
	s.dlfilesh = http.StripPrefix("/download/", http.HandlerFunc(s.serveDownloadFiles))
	s.rssh = http.HandlerFunc(s.serveRSS)

	//scraper
	s.scraper = &scraper.Handler{
		Log: s.Debug, Debug: s.Debug,
		Headers: map[string]string{
			//we're a trusty browser :)
			"User-Agent": scraperUA,
		},
	}
	if err := s.scraper.LoadConfig(defaultSearchConfig); err != nil {
		log.Fatal(err)
	}
	s.searchProviders = &s.scraper.Config //share scraper config with web frontend
	s.scraperh = http.StripPrefix("/search", s.scraper)

	// sync config from cmd arg to viper
	viper.SetDefault("ProxyURL", s.ProxyURL)

	//torrent engine
	s.engine = engine.New(s)
	c, err := engine.InitConf(&s.ConfigPath)
	if err != nil {
		return err
	}
	c.EngineDebug = s.DebugTorrent

	// write cloud-torrent.yaml at the same dir with -c conf and exit
	if s.ConvYAML {
		cf := viper.ConfigFileUsed()
		if !strings.HasSuffix(cf, ".yaml") {
			log.Println("[config] current file path: ", cf)

			// replace orig config file ext with ".yaml"
			ymlcf := cf[:len(cf)-len(path.Ext(cf))] + ".yaml"
			if err := c.WriteYaml(ymlcf); err != nil {
				return err
			}
			return fmt.Errorf("config file converted and written to: %s", ymlcf)
		}
		return fmt.Errorf("config file is already yaml: %s", cf)
	}

	if err := detectDiskStat(c.DownloadDirectory); err != nil {
		return err
	}

	// engine configure
	s.state.Stats.System.diskDirPath = c.DownloadDirectory
	s.state.UseQueue = (c.MaxConcurrentTask > 0)
	s.engineConfig = c
	s.tpl.AllowRuntimeConfigure = c.AllowRuntimeConfigure
	if err := s.engine.Configure(c); err != nil {
		return err
	}
	s.state.Torrents = s.engine.GetTorrents()

	if s.Debug {
		viper.Debug()
		log.Printf("Effective Config: %#v", *c)
	}

	if err := s.engine.ParseTrackerList(); err != nil {
		log.Println("UpdateTrackers err", err)
	}
	s.backgroundRoutines()

	if s.Open && !isListenOnUnix {
		go func() {
			proto := "http"
			if isTLS {
				proto += "s"
			}
			time.Sleep(1 * time.Second)
			common.FancyHandleError(open.Run(fmt.Sprintf("%s://localhost:%d", proto, s.Port)))
		}()
	}

	// restful API server
	if s.RestAPI != "" {
		go func() {
			restServer := http.Server{
				Addr: s.RestAPI,
				Handler: requestlog.Wrap(
					httpmiddleware.RealIP(
						http.Handler(http.HandlerFunc(s.restAPIhandle)),
					),
				),
			}
			log.Println("[RestAPI] listening at ", s.RestAPI)
			if err := restServer.ListenAndServe(); err != nil {
				log.Println("[RestAPI] err ", err)
			}
		}()
	}

	//define handler chain, from last to first
	h := http.Handler(http.HandlerFunc(s.webHandle))
	//gzip
	h = httpmiddleware.RealIP(h)
	h = httpmiddleware.Liveness(h)

	// dont enable gzip handler if certantlly we are behind a web server
	if !isListenOnUnix {
		gzipWrap, _ := gziphandler.NewGzipLevelAndMinSize(gzip.DefaultCompression, 1024)
		h = gzipWrap(h)
	}

	//auth
	if s.Auth != "" {
		user := s.Auth
		pass := ""
		if s := strings.SplitN(s.Auth, ":", 2); len(s) == 2 {
			user = s[0]
			pass = s[1]
		}
		h = cookieauth.New().SetUserPass(user, pass).Wrap(h)
		log.Printf("Enabled HTTP authentication")
	}
	if s.ReqLog {
		h = requestlog.Wrap(h)
	}

	server := http.Server{
		//handler stack
		Handler: h,
	}

	//serve!
	var listener net.Listener
	if isListenOnUnix {
		sockPath := s.Listen[5:]
		if _, err := os.Stat(sockPath); !errors.Is(err, os.ErrNotExist) {
			log.Println("Listening sock exists, removing", sockPath)
			os.Remove(sockPath)
		}
		log.Println("Listening at", s.Listen)
		listener, err = net.Listen("unix", sockPath)
		if err != nil {
			log.Fatalln("Failed listening", err)
		}
		if um, err := strconv.ParseInt(s.UnixPerm, 8, 0); err == nil {
			uxmod := os.FileMode(um)
			log.Println("Listening DomainSocket mode change to:", uxmod.String(), s.UnixPerm)
			common.HandleError(os.Chmod(sockPath, uxmod))
		}
	} else {
		log.Println("Listening at", s.Listen)
		listener, err = net.Listen("tcp", s.Listen)
		if err != nil {
			log.Fatalln("Failed listening", err)
		}
		if isTLS {
			return server.ServeTLS(listener, s.CertPath, s.KeyPath)
		}
	}
	return server.Serve(listener)
}

func init() {
	log = stdlog.New(os.Stdout, "[server]", stdlog.LstdFlags|stdlog.Lmsgprefix)
}

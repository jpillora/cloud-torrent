package server

import (
	"cloud-torrent/server/httpmiddleware"
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/NYTimes/gziphandler"
	"github.com/boypt/scraper"
	"github.com/jpillora/cloud-torrent/engine"
	ctstatic "github.com/jpillora/cloud-torrent/static"
	"github.com/jpillora/cookieauth"
	"github.com/jpillora/requestlog"
	"github.com/jpillora/velox"
	"github.com/mmcdole/gofeed"
	"github.com/radovskyb/watcher"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/viper"
)

const (
	cacheSavedPrefix = "_CLDAUTOSAVED_"
	scraperUA        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/57.0.2987.133 Safari/537.36"
)

var (
	//ErrDiskSpace raised if disk space not enough
	ErrDiskSpace = errors.New("not enough disk space")
)

//Server is the "State" portion of the diagram
type Server struct {
	//config
	Title          string `opts:"help=Title of this instance,env=TITLE"`
	Port           int    `opts:"help=Listening port,env=PORT"`
	Host           string `opts:"help=Listening interface (default all)"`
	Auth           string `opts:"help=Optional basic auth in form 'user:password',env=AUTH"`
	ProxyURL       string `opts:"help=Proxy url,env=PROXY_URL"`
	ConfigPath     string `opts:"help=Configuration file path (default /etc/cloud-torrent.yaml)"`
	KeyPath        string `opts:"help=TLS Key file path"`
	CertPath       string `opts:"help=TLS Certicate file path,short=r"`
	RestAPI        string `opts:"help=Listen on a trusted port accepts /api/ requests (eg. localhost:3001),env=RESTAPI"`
	Log            bool   `opts:"help=Enable request logging"`
	Open           bool   `opts:"help=Open now with your default browser"`
	DisableLogTime bool   `opts:"help=Don't print timestamp in log"`
	Debug          bool   `opts:"help=Debug app"`
	DebugTorrent   bool   `opts:"help=Debug torrent engine"`
	ConvYAML       bool   `opts:"help=Convert old json config to yaml format."`
	mainAddr       string
	isPendingBoot  bool

	//http handlers
	files, static, rssh http.Handler
	scraper             *scraper.Handler
	scraperh            http.Handler

	//file watcher
	watcher *watcher.Watcher

	//torrent engine
	engine *engine.Engine
	state  struct {
		velox.State
		sync.Mutex
		Config          engine.Config
		SearchProviders scraper.Config
		Downloads       *fsNode
		rssMark         map[string]string
		rssCache        []*gofeed.Item
		LatestRSSGuid   string
		Torrents        map[string]*engine.Torrent
		Users           map[string]string
		Stats           struct {
			Title   string
			Version string
			Runtime string
			Uptime  time.Time
			System  stats
		}
	}
}

// GetRestAPI used by engine doneCmd
func (s *Server) GetRestAPI() string {
	return s.RestAPI
}

// GetIsPendingBoot used by engine doneCmd
func (s *Server) GetIsPendingBoot() bool {
	return s.isPendingBoot
}

// Run the server
func (s *Server) Run(version string) error {
	isTLS := s.CertPath != "" || s.KeyPath != "" //poor man's XOR
	if isTLS && (s.CertPath == "" || s.KeyPath == "") {
		return fmt.Errorf("You must provide both key and cert paths")
	}
	s.state.Stats.Title = s.Title
	s.state.Stats.Version = version
	s.state.Stats.Runtime = strings.TrimPrefix(runtime.Version(), "go")
	s.state.Stats.Uptime = time.Now()
	s.state.Stats.System.pusher = velox.Pusher(&s.state)
	//init maps
	s.state.Users = make(map[string]string)
	s.state.rssMark = make(map[string]string)

	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.files = http.HandlerFunc(s.serveFiles)
	s.static = ctstatic.FileSystemHandler()
	s.rssh = http.HandlerFunc(s.serveRSS)

	// isPendingBoot last for 30s, doneCMD won't be triggered
	s.isPendingBoot = true
	go func() {
		<-time.After(time.Second * 30)
		s.isPendingBoot = false
	}()

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
	s.state.SearchProviders = s.scraper.Config //share scraper config with web frontend
	s.scraperh = http.StripPrefix("/search", s.scraper)

	// sync config from cmd arg to viper
	viper.SetDefault("ProxyURL", s.ProxyURL)

	//torrent engine
	s.engine = engine.New(s)
	c, err := engine.InitConf(s.ConfigPath)
	if err != nil {
		return err
	}

	// write cloud-torrent.yaml at the same dir with -c conf and exit
	if s.ConvYAML {
		cf := viper.ConfigFileUsed()
		log.Println("[config] current file path: ", cf)
		// replace orig config file ext with ".yaml"
		ymlcf := cf[:len(cf)-len(path.Ext(cf))] + ".yaml"
		if err := viper.WriteConfigAs(ymlcf); err != nil {
			return err
		}
		return fmt.Errorf("Config file converted and written to: %s", ymlcf)
	}

	if err := detectDiskStat(c.DownloadDirectory); err != nil {
		return err
	}

	// engine configure
	s.state.Config = *c
	if err := s.engine.Configure(&s.state.Config); err != nil {
		return err
	}

	if s.Debug {
		viper.Debug()
		log.Printf("Effective Config: %#v", s.state.Config)
	}

	s.backgroundRoutines()
	s.torrentWatcher()

	s.mainAddr = fmt.Sprintf("%s:%d", s.Host, s.Port)
	proto := "http"
	if isTLS {
		proto += "s"
	}
	if s.Open {
		go func() {
			time.Sleep(1 * time.Second)
			open.Run(fmt.Sprintf("%s://localhost:%d", proto, s.Port))
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
	gzipWrap, _ := gziphandler.NewGzipLevelAndMinSize(gzip.DefaultCompression, 0)
	h = gzipWrap(h)
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
	if s.Log {
		h = requestlog.Wrap(h)
	}
	//serve!
	server := http.Server{
		//disable http2 due to velox bug
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		//address
		Addr: s.mainAddr,
		//handler stack
		Handler: h,
	}
	listenLog := "0.0.0.0" + s.mainAddr
	if !strings.HasPrefix(s.mainAddr, ":") {
		listenLog = s.mainAddr
	}
	log.Printf("Listening at %s://%s", proto, listenLog)
	if isTLS {
		return server.ListenAndServeTLS(s.CertPath, s.KeyPath)
	}
	return server.ListenAndServe()
}

func fileExists(fn string) bool {
	stat, err := os.Stat(fn)
	return stat != nil && err == nil
}

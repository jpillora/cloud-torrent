package cloudtorrent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	"github.com/jpillora/cloud-torrent/cloudtorrent/fs"
	"github.com/jpillora/cloud-torrent/cloudtorrent/fs/disk"
	"github.com/jpillora/cloud-torrent/cloudtorrent/fs/dropbox"
	"github.com/jpillora/cloud-torrent/cloudtorrent/fs/torrent"
	"github.com/jpillora/cloud-torrent/cloudtorrent/static"
	"github.com/jpillora/cookieauth"
	"github.com/jpillora/scraper/scraper"
	"github.com/jpillora/velox"
	"github.com/skratchdot/open-golang/open"
)

//App is the core cloudtorrent application
type App struct {
	//command-line options
	Title      string `help:"Title of this instance" env:"TITLE"`
	Port       int    `help:"Listening port" env:"PORT"`
	Host       string `help:"Listening interface (default all)"`
	Auth       string `help:"Optional basic auth in form 'user:password'" env:"AUTH"`
	ConfigPath string `help:"Configuration file path"`
	KeyPath    string `help:"TLS Key file path"`
	CertPath   string `help:"TLS Certicate file path" short:"r"`
	Log        bool   `help:"Enable request logging"`
	Open       bool   `help:"Open now with your default browser"`
	//internal state
	config        AppConfig
	files, static http.Handler
	scraper       *scraper.Handler
	scraperh      http.Handler
	auth          *cookieauth.CookieAuth
	fileSystems   map[string]fs.FS
	prevConfigs   rawMessages
	//velox (browser) state
	state struct {
		velox.State
		sync.Mutex
		SearchProviders scraper.Config
		Configurations  map[string]interface{}
		FSS             map[string]*FileSystemState
		Users           map[string]time.Time
		Stats           struct {
			Title   string
			Version string
			Runtime string
			Uptime  time.Time
		}
	}
}

func (a *App) Run(version string) error {
	logf("run...")
	//validate config
	tls := a.CertPath != "" || a.KeyPath != "" //poor man's XOR
	if tls && (a.CertPath == "" || a.KeyPath == "") {
		return fmt.Errorf("You must provide both key and cert paths")
	}
	a.config.Title = a.Title
	if auth := strings.SplitN(a.Auth, ":", 2); len(auth) == 2 {
		a.config.User = auth[0]
		a.config.Pass = auth[1]
	}
	//prepare initial empty configs
	a.prevConfigs = rawMessages{}
	cfgs := rawMessages{
		"App": EmptyConfig,
	}
	//system statistics
	a.state.Stats.Title = a.Title
	a.state.Stats.Version = version
	a.state.Stats.Runtime = strings.TrimPrefix(runtime.Version(), "go")
	a.state.Stats.Uptime = time.Now()
	//app state
	a.state.Configurations = map[string]interface{}{}
	a.state.FSS = map[string]*FileSystemState{}
	a.state.Users = map[string]time.Time{}
	//init filesystems
	a.fileSystems = map[string]fs.FS{}
	for _, fs := range []fs.FS{
		torrent.New(),
		disk.New(),
		dropbox.New(),
	} {
		n := fs.Name()
		if _, ok := a.fileSystems[n]; ok {
			return errors.New("duplicate fs: " + n)
		}
		cfgs[n] = EmptyConfig
		a.fileSystems[n] = fs
		a.state.FSS[n] = &FileSystemState{Enabled: true}
	}

	//app handlers
	a.auth = cookieauth.New()
	//static will use a the local static/ dir if it exists,
	//otherwise will use the embedded files
	a.static = static.FileSystemHandler()
	//scraper has initial config stored as a JSON asset
	a.scraper = &scraper.Handler{Log: false}
	if err := a.scraper.LoadConfig(static.MustAsset("files/misc/search.json")); err != nil {
		log.Fatal(err)
	}
	a.state.SearchProviders = a.scraper.Config //share scraper config
	a.scraperh = http.StripPrefix("/search", a.scraper)
	//configure
	if _, err := os.Stat(a.ConfigPath); err == nil {
		if b, err := ioutil.ReadFile(a.ConfigPath); err != nil {
			return fmt.Errorf("Read configurations error: %s", err)
		} else if len(b) == 0 {
			//ignore empty file
		} else if err := json.Unmarshal(b, &cfgs); err != nil {
			return fmt.Errorf("initial configure failed: %s", err)
		}
	}
	//initial configure
	if err := a.configureAll(cfgs); err != nil {
		return fmt.Errorf("initial configure failed: %s", err)
	}
	//start server
	host := a.Host
	if host == "" {
		host = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", host, a.Port)
	proto := "http"
	if tls {
		proto += "s"
	}
	if a.Open {
		h := host
		if h == "0.0.0.0" {
			h = "localhost"
		}
		go func() {
			time.Sleep(1 * time.Second)
			open.Run(fmt.Sprintf("%s://%s:%d", proto, h, a.Port))
		}()
	}
	//top layer is the app handler
	h := a.routes()
	//serve tls/plain http
	logf("Listening at %s://%s", proto, addr)
	if tls {
		return http.ListenAndServeTLS(addr, a.CertPath, a.KeyPath, h)
	} else {
		return http.ListenAndServe(addr, h)
	}
}

//startFSSync runs once after the first
//successful configure, then loops fs.Update()
//forever, with exponential backoff on failures.
func (a *App) startFSSync(f fs.FS) {
	name := f.Name()
	updates := make(chan fs.Node)
	fsstate := a.state.FSS[name]
	//monitor and sync updates
	go func() {
		for node := range updates {
			a.state.Lock()
			log.Printf("[%s] updated", name)
			fsstate.Root = &fs.JSONNode{Node: node}
			a.state.Unlock()
			a.state.Push()
		}
	}()
	//sync loop forever
	go func() {
		b := backoff.Backoff{Max: 2 * time.Minute}
		for {
			//retrieve updates
			err := f.Update(updates)
			e := ""
			d := 30 * time.Second
			if err == nil {
				b.Reset()
			} else {
				log.Printf("[%s] sync failed: %s", name, err)
				e = err.Error()
				d = b.Duration()
			}
			//show result
			a.state.Lock()
			fsstate.Error = e
			a.state.Unlock()
			a.state.Push()
			//retry after sleep
			time.Sleep(d)
		}
	}()
	log.Printf("[%s] Sync started", name)
}

func logf(format string, args ...interface{}) {
	log.Printf("[App] "+format, args...)
}

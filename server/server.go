package server

// Server is the cloud-torrent HTTP server. Its exported fields double as the
// command-line flags, parsed by github.com/jpillora/opts (see main.go).
type Server struct {
	Title      string `help:"Title of this instance" env:"TITLE"`
	Port       int    `help:"Listening port" env:"PORT"`
	Host       string `help:"Listening interface (default all)"`
	Auth       string `help:"Optional basic auth in form 'user:password'" env:"AUTH"`
	ConfigPath string `help:"Configuration file path"`
	KeyPath    string `help:"TLS Key file path"`
	CertPath   string `help:"TLS Certificate file path" short:"r"`
	Log        bool   `help:"Enable request logging"`
	Open       bool   `help:"Open now with your default browser"`
}

// Run starts the HTTP server. This is a skeleton: the torrent engine, realtime
// state sync, action API, and static asset serving are wired up in later tasks.
func (s *Server) Run(version string) error {
	return nil
}

// Mounts a FUSE filesystem backed by torrents and magnet links.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"bazil.org/fuse"
	fusefs "bazil.org/fuse/fs"
	_ "github.com/anacrolix/envpprof"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/fs"
	"github.com/anacrolix/torrent/util/dirwatch"
)

var (
	torrentPath = flag.String("torrentPath", func() string {
		_user, err := user.Current()
		if err != nil {
			log.Fatal(err)
		}
		return filepath.Join(_user.HomeDir, ".config/transmission/torrents")
	}(), "torrent files in this location describe the contents of the mounted filesystem")
	downloadDir = flag.String("downloadDir", "", "location to save torrent data")
	mountDir    = flag.String("mountDir", "", "location the torrent contents are made available")

	disableTrackers = flag.Bool("disableTrackers", false, "disables trackers")
	testPeer        = flag.String("testPeer", "", "the address for a test peer")
	readaheadBytes  = flag.Int64("readaheadBytes", 10*1024*1024, "bytes to readahead in each torrent from the last read piece")
	listenAddr      = flag.String("listenAddr", ":6882", "incoming connection address")

	testPeerAddr *net.TCPAddr
)

func resolveTestPeerAddr() {
	if *testPeer == "" {
		return
	}
	var err error
	testPeerAddr, err = net.ResolveTCPAddr("tcp4", *testPeer)
	if err != nil {
		log.Fatal(err)
	}
}

func exitSignalHandlers(fs *torrentfs.TorrentFS) {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	for {
		<-c
		fs.Destroy()
		err := fuse.Unmount(*mountDir)
		if err != nil {
			log.Print(err)
		}
	}
}

func addTestPeer(client *torrent.Client) {
	for _, t := range client.Torrents() {
		if testPeerAddr != nil {
			if err := t.AddPeers([]torrent.Peer{{
				IP:   testPeerAddr.IP,
				Port: testPeerAddr.Port,
			}}); err != nil {
				log.Print(err)
			}
		}
	}
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 {
		os.Stderr.WriteString("one does not simply pass positional args\n")
		os.Exit(2)
	}
	if *mountDir == "" {
		os.Stderr.WriteString("y u no specify mountpoint?\n")
		os.Exit(2)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	conn, err := fuse.Mount(*mountDir)
	if err != nil {
		log.Fatal(err)
	}
	defer fuse.Unmount(*mountDir)
	// TODO: Think about the ramifications of exiting not due to a signal.
	defer conn.Close()
	client, err := torrent.NewClient(&torrent.Config{
		DataDir:         *downloadDir,
		DisableTrackers: *disableTrackers,
		ListenAddr:      *listenAddr,
		NoUpload:        true, // Ensure that downloads are responsive.
	})
	if err != nil {
		log.Fatal(err)
	}
	// This is naturally exported via GOPPROF=http.
	http.DefaultServeMux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		client.WriteStatus(w)
	})
	dw, err := dirwatch.New(*torrentPath)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for ev := range dw.Events {
			switch ev.Change {
			case dirwatch.Added:
				if ev.TorrentFilePath != "" {
					_, err := client.AddTorrentFromFile(ev.TorrentFilePath)
					if err != nil {
						log.Printf("error adding torrent to client: %s", err)
					}
				} else if ev.MagnetURI != "" {
					_, err := client.AddMagnet(ev.MagnetURI)
					if err != nil {
						log.Printf("error adding magnet: %s", err)
					}
				}
			case dirwatch.Removed:
				T, ok := client.Torrent(ev.InfoHash)
				if !ok {
					break
				}
				T.Drop()
			}
		}
	}()
	resolveTestPeerAddr()
	fs := torrentfs.New(client)
	go exitSignalHandlers(fs)
	go func() {
		for {
			addTestPeer(client)
			time.Sleep(10 * time.Second)
		}
	}()

	if err := fusefs.Serve(conn, fs); err != nil {
		log.Fatal(err)
	}
	<-conn.Ready
	if err := conn.MountError; err != nil {
		log.Fatal(err)
	}
}

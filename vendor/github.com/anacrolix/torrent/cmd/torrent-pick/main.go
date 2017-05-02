// Downloads torrents from the command-line.
package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"time"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/missinggo"
	"github.com/dustin/go-humanize"
	"github.com/jessevdk/go-flags"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// fmt.Fprintf(os.Stderr, "Usage: %s \n", os.Args[0])

func resolvedPeerAddrs(ss []string) (ret []torrent.Peer, err error) {
	for _, s := range ss {
		var addr *net.TCPAddr
		addr, err = net.ResolveTCPAddr("tcp", s)
		if err != nil {
			return
		}
		ret = append(ret, torrent.Peer{
			IP:   addr.IP,
			Port: addr.Port,
		})
	}
	return
}

func bytesCompleted(tc *torrent.Client) (ret int64) {
	for _, t := range tc.Torrents() {
		if t.Info() != nil {
			ret += t.BytesCompleted()
		}
	}
	return
}

// Returns an estimate of the total bytes for all torrents.
func totalBytesEstimate(tc *torrent.Client) (ret int64) {
	var noInfo, hadInfo int64
	for _, t := range tc.Torrents() {
		info := t.Info()
		if info == nil {
			noInfo++
			continue
		}
		ret += info.TotalLength()
		hadInfo++
	}
	if hadInfo != 0 {
		// Treat each torrent without info as the average of those with,
		// rounded up.
		ret += (noInfo*ret + hadInfo - 1) / hadInfo
	}
	return
}

func progressLine(tc *torrent.Client) string {
	return fmt.Sprintf("\033[K%s / %s\r", humanize.Bytes(uint64(bytesCompleted(tc))), humanize.Bytes(uint64(totalBytesEstimate(tc))))
}

func dstFileName(picked string) string {
	parts := strings.Split(picked, "/")
	return parts[len(parts)-1]
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var rootGroup struct {
		Client    torrent.Config `group:"Client Options"`
		TestPeers []string       `long:"test-peer" description:"address of peer to inject to every torrent"`
		Pick      string         `long:"pick" description:"filename to pick"`
	}
	// Don't pass flags.PrintError because it's inconsistent with printing.
	// https://github.com/jessevdk/go-flags/issues/132
	parser := flags.NewParser(&rootGroup, flags.HelpFlag|flags.PassDoubleDash)
	parser.Usage = "[OPTIONS] (magnet URI or .torrent file path)..."
	posArgs, err := parser.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", "Download from the BitTorrent network.\n\n")
		fmt.Println(err)
		os.Exit(2)
	}
	log.Printf("File to pick: %s", rootGroup.Pick)

	testPeers, err := resolvedPeerAddrs(rootGroup.TestPeers)
	if err != nil {
		log.Fatal(err)
	}

	if len(posArgs) == 0 {
		fmt.Fprintln(os.Stderr, "no torrents specified")
		return
	}

	tmpdir, err := ioutil.TempDir("", "torrent-pick-")
	if err != nil {
		log.Fatal(err)
	}

	defer os.RemoveAll(tmpdir)

	rootGroup.Client.DataDir = tmpdir

	client, err := torrent.NewClient(&rootGroup.Client)
	if err != nil {
		log.Fatalf("error creating client: %s", err)
	}
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		client.WriteStatus(w)
	})
	defer client.Close()

	dstName := dstFileName(rootGroup.Pick)

	f, err := os.Create(dstName)
	if err != nil {
		log.Fatal(err)
	}
	dstWriter := bufio.NewWriter(f)

	done := make(chan struct{})
	for _, arg := range posArgs {
		t := func() *torrent.Torrent {
			if strings.HasPrefix(arg, "magnet:") {
				t, err := client.AddMagnet(arg)
				if err != nil {
					log.Fatalf("error adding magnet: %s", err)
				}
				return t
			} else {
				metaInfo, err := metainfo.LoadFromFile(arg)
				if err != nil {
					log.Fatal(err)
				}
				t, err := client.AddTorrent(metaInfo)
				if err != nil {
					log.Fatal(err)
				}
				return t
			}
		}()
		t.AddPeers(testPeers)

		go func() {
			defer close(done)
			<-t.GotInfo()
			for _, file := range t.Files() {
				if file.DisplayPath() != rootGroup.Pick {
					continue
				}
				srcReader := missinggo.NewSectionReadSeeker(t.NewReader(), file.Offset(), file.Length())
				io.Copy(dstWriter, srcReader)
				return
			}
			log.Print("file not found")
		}()
	}

	ticker := time.NewTicker(time.Second)
waitDone:
	for {
		select {
		case <-done:
			break waitDone
		case <-ticker.C:
			os.Stdout.WriteString(progressLine(client))
		}
	}
	if rootGroup.Client.Seed {
		select {}
	}
}

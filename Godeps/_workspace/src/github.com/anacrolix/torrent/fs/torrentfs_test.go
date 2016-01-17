package torrentfs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"bazil.org/fuse"
	fusefs "bazil.org/fuse/fs"
	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/missinggo"
	"github.com/stretchr/testify/assert"
	netContext "golang.org/x/net/context"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/data/mmap"
	"github.com/anacrolix/torrent/internal/testutil"
	"github.com/anacrolix/torrent/metainfo"
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
}

func TestTCPAddrString(t *testing.T) {
	l, err := net.Listen("tcp4", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	c, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ras := c.RemoteAddr().String()
	ta := &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: missinggo.AddrPort(l.Addr()),
	}
	s := ta.String()
	if ras != s {
		t.FailNow()
	}
}

type testLayout struct {
	BaseDir   string
	MountDir  string
	Completed string
	Metainfo  *metainfo.MetaInfo
}

func (me *testLayout) Destroy() error {
	return os.RemoveAll(me.BaseDir)
}

func newGreetingLayout() (tl testLayout, err error) {
	tl.BaseDir, err = ioutil.TempDir("", "torrentfs")
	if err != nil {
		return
	}
	tl.Completed = filepath.Join(tl.BaseDir, "completed")
	os.Mkdir(tl.Completed, 0777)
	tl.MountDir = filepath.Join(tl.BaseDir, "mnt")
	os.Mkdir(tl.MountDir, 0777)
	name := testutil.CreateDummyTorrentData(tl.Completed)
	metaInfoBuf := &bytes.Buffer{}
	testutil.CreateMetaInfo(name, metaInfoBuf)
	tl.Metainfo, err = metainfo.Load(metaInfoBuf)
	return
}

// Unmount without first killing the FUSE connection while there are FUSE
// operations blocked inside the filesystem code.
func TestUnmountWedged(t *testing.T) {
	layout, err := newGreetingLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := layout.Destroy()
		if err != nil {
			t.Log(err)
		}
	}()
	client, err := torrent.NewClient(&torrent.Config{
		DataDir:         filepath.Join(layout.BaseDir, "incomplete"),
		DisableTrackers: true,
		NoDHT:           true,
		ListenAddr:      "redonk",
		DisableTCP:      true,
		DisableUTP:      true,

		NoDefaultBlocklist: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.AddTorrent(layout.Metainfo)
	fs := New(client)
	fuseConn, err := fuse.Mount(layout.MountDir)
	if err != nil {
		msg := fmt.Sprintf("error mounting: %s", err)
		if strings.Contains(err.Error(), "fuse") || err.Error() == "exit status 71" {
			t.Skip(msg)
		}
		t.Fatal(msg)
	}
	go func() {
		server := fusefs.New(fuseConn, &fusefs.Config{
			Debug: func(msg interface{}) {
				t.Log(msg)
			},
		})
		server.Serve(fs)
	}()
	<-fuseConn.Ready
	if err := fuseConn.MountError; err != nil {
		t.Fatalf("mount error: %s", err)
	}
	// Read the greeting file, though it will never be available. This should
	// "wedge" FUSE, requiring the fs object to be forcibly destroyed. The
	// read call will return with a FS error.
	go func() {
		_, err := ioutil.ReadFile(filepath.Join(layout.MountDir, layout.Metainfo.Info.Name))
		if err == nil {
			t.Fatal("expected error reading greeting")
		}
	}()

	// Wait until the read has blocked inside the filesystem code.
	fs.mu.Lock()
	for fs.blockedReads != 1 {
		fs.event.Wait()
	}
	fs.mu.Unlock()

	fs.Destroy()

	for {
		err = fuse.Unmount(layout.MountDir)
		if err != nil {
			t.Logf("error unmounting: %s", err)
			time.Sleep(time.Millisecond)
		} else {
			break
		}
	}

	err = fuseConn.Close()
	if err != nil {
		t.Fatalf("error closing fuse conn: %s", err)
	}
}

func TestDownloadOnDemand(t *testing.T) {
	layout, err := newGreetingLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.Destroy()
	seeder, err := torrent.NewClient(&torrent.Config{
		DataDir:         layout.Completed,
		DisableTrackers: true,
		NoDHT:           true,
		ListenAddr:      ":0",
		Seed:            true,

		NoDefaultBlocklist: true,
		// Ensure that the metainfo is obtained over the wire, since we added
		// the torrent to the seeder by magnet.
		DisableMetainfoCache: true,
	})
	if err != nil {
		t.Fatalf("error creating seeder client: %s", err)
	}
	seeder.SetIPBlockList(nil)
	defer seeder.Close()
	http.HandleFunc("/seeder", func(w http.ResponseWriter, req *http.Request) {
		seeder.WriteStatus(w)
	})
	_, err = seeder.AddMagnet(fmt.Sprintf("magnet:?xt=urn:btih:%x", layout.Metainfo.Info.Hash))
	if err != nil {
		t.Fatal(err)
	}
	leecher, err := torrent.NewClient(&torrent.Config{
		DisableTrackers: true,
		NoDHT:           true,
		ListenAddr:      ":0",
		DisableTCP:      true,

		NoDefaultBlocklist: true,

		TorrentDataOpener: func(info *metainfo.Info) torrent.Data {
			ret, _ := mmap.TorrentData(info, filepath.Join(layout.BaseDir, "download"))
			return ret
		},

		// This can be used to check if clients can connect to other clients
		// with the same ID.

		// PeerID: seeder.PeerID(),
	})
	leecher.SetIPBlockList(nil)
	http.HandleFunc("/leecher", func(w http.ResponseWriter, req *http.Request) {
		leecher.WriteStatus(w)
	})
	defer leecher.Close()
	leecherTorrent, _ := leecher.AddTorrent(layout.Metainfo)
	leecherTorrent.AddPeers([]torrent.Peer{func() torrent.Peer {
		_, port, err := net.SplitHostPort(seeder.ListenAddr().String())
		if err != nil {
			panic(err)
		}
		portInt64, err := strconv.ParseInt(port, 0, 0)
		if err != nil {
			panic(err)
		}
		return torrent.Peer{
			IP: func() net.IP {
				ret, _ := net.ResolveIPAddr("ip", "localhost")
				return ret.IP
			}(),
			Port: int(portInt64),
		}
	}()})
	fs := New(leecher)
	defer fs.Destroy()
	root, _ := fs.Root()
	node, _ := root.(fusefs.NodeStringLookuper).Lookup(netContext.Background(), "greeting")
	var attr fuse.Attr
	node.Attr(netContext.Background(), &attr)
	size := attr.Size
	resp := &fuse.ReadResponse{
		Data: make([]byte, size),
	}
	node.(fusefs.HandleReader).Read(netContext.Background(), &fuse.ReadRequest{
		Size: int(size),
	}, resp)
	content := resp.Data
	if string(content) != testutil.GreetingFileContents {
		t.Fatalf("%q != %q", string(content), testutil.GreetingFileContents)
	}
}

func TestIsSubPath(t *testing.T) {
	for _, case_ := range []struct {
		parent, child string
		is            bool
	}{
		{"", "", false},
		{"", "/", true},
		{"a/b", "a/bc", false},
		{"a/b", "a/b", false},
		{"a/b", "a/b/c", true},
		{"a/b", "a//b", false},
	} {
		assert.Equal(t, case_.is, isSubPath(case_.parent, case_.child))
	}
}

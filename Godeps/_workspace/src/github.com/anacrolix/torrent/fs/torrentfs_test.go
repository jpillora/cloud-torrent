package torrentfs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bazil.org/fuse"
	fusefs "bazil.org/fuse/fs"
	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/missinggo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	netContext "golang.org/x/net/context"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/internal/testutil"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
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
	require.NoError(t, err)
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
	})
	require.NoError(t, err)
	defer client.Close()
	_, err = client.AddTorrent(layout.Metainfo)
	require.NoError(t, err)
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
	require.NoError(t, err)
	defer layout.Destroy()
	seeder, err := torrent.NewClient(&torrent.Config{
		DataDir:         layout.Completed,
		DisableTrackers: true,
		NoDHT:           true,
		ListenAddr:      "localhost:0",
		Seed:            true,
		// Ensure that the metainfo is obtained over the wire, since we added
		// the torrent to the seeder by magnet.
		DisableMetainfoCache: true,
	})
	require.NoError(t, err)
	defer seeder.Close()
	testutil.ExportStatusWriter(seeder, "s")
	_, err = seeder.AddMagnet(fmt.Sprintf("magnet:?xt=urn:btih:%s", layout.Metainfo.Info.Hash.HexString()))
	require.NoError(t, err)
	leecher, err := torrent.NewClient(&torrent.Config{
		DisableTrackers: true,
		NoDHT:           true,
		ListenAddr:      "localhost:0",
		DisableTCP:      true,
		DefaultStorage:  storage.NewMMap(filepath.Join(layout.BaseDir, "download")),
		// This can be used to check if clients can connect to other clients
		// with the same ID.
		// PeerID: seeder.PeerID(),
	})
	require.NoError(t, err)
	testutil.ExportStatusWriter(leecher, "l")
	defer leecher.Close()
	leecherTorrent, _ := leecher.AddTorrent(layout.Metainfo)
	leecherTorrent.AddPeers([]torrent.Peer{
		torrent.Peer{
			IP:   missinggo.AddrIP(seeder.ListenAddr()),
			Port: missinggo.AddrPort(seeder.ListenAddr()),
		},
	})
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
	assert.EqualValues(t, testutil.GreetingFileContents, resp.Data)
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

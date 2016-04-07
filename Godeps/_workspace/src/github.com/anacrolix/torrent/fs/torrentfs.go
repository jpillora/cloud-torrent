package torrentfs

import (
	"expvar"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"bazil.org/fuse"
	fusefs "bazil.org/fuse/fs"
	"golang.org/x/net/context"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

const (
	defaultMode = 0555
)

var (
	torrentfsReadRequests        = expvar.NewInt("torrentfsReadRequests")
	torrentfsDelayedReadRequests = expvar.NewInt("torrentfsDelayedReadRequests")
	interruptedReads             = expvar.NewInt("interruptedReads")
)

type TorrentFS struct {
	Client       *torrent.Client
	destroyed    chan struct{}
	mu           sync.Mutex
	blockedReads int
	event        sync.Cond
}

var (
	_ fusefs.FSDestroyer = &TorrentFS{}

	_ fusefs.NodeForgetter      = rootNode{}
	_ fusefs.HandleReadDirAller = rootNode{}
	_ fusefs.HandleReadDirAller = dirNode{}
)

type rootNode struct {
	fs *TorrentFS
}

type node struct {
	path     string
	metadata *metainfo.InfoEx
	FS       *TorrentFS
	t        *torrent.Torrent
}

type fileNode struct {
	node
	size          uint64
	TorrentOffset int64
}

func (fn fileNode) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Size = fn.size
	attr.Mode = defaultMode
	return nil
}

func (n *node) fsPath() string {
	return "/" + n.metadata.Name + "/" + n.path
}

func blockingRead(ctx context.Context, fs *TorrentFS, t *torrent.Torrent, off int64, p []byte) (n int, err error) {
	fs.mu.Lock()
	fs.blockedReads++
	fs.event.Broadcast()
	fs.mu.Unlock()
	var (
		_n   int
		_err error
	)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		r := t.NewReader()
		defer r.Close()
		_, _err = r.Seek(off, os.SEEK_SET)
		if _err != nil {
			return
		}
		_n, _err = io.ReadFull(r, p)
	}()
	select {
	case <-readDone:
		n = _n
		err = _err
	case <-fs.destroyed:
		err = fuse.EIO
	case <-ctx.Done():
		err = fuse.EINTR
	}
	fs.mu.Lock()
	fs.blockedReads--
	fs.event.Broadcast()
	fs.mu.Unlock()
	return
}

func readFull(ctx context.Context, fs *TorrentFS, t *torrent.Torrent, off int64, p []byte) (n int, err error) {
	for len(p) != 0 {
		var nn int
		nn, err = blockingRead(ctx, fs, t, off, p)
		if err != nil {
			break
		}
		n += nn
		off += int64(nn)
		p = p[nn:]
	}
	return
}

func (fn fileNode) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	torrentfsReadRequests.Add(1)
	if req.Dir {
		panic("read on directory")
	}
	size := req.Size
	fileLeft := int64(fn.size) - req.Offset
	if fileLeft < 0 {
		fileLeft = 0
	}
	if fileLeft < int64(size) {
		size = int(fileLeft)
	}
	resp.Data = resp.Data[:size]
	if len(resp.Data) == 0 {
		return nil
	}
	torrentOff := fn.TorrentOffset + req.Offset
	n, err := readFull(ctx, fn.FS, fn.t, torrentOff, resp.Data)
	if err != nil {
		return err
	}
	if n != size {
		panic(fmt.Sprintf("%d < %d", n, size))
	}
	return nil
}

type dirNode struct {
	node
}

var (
	_ fusefs.HandleReadDirAller = dirNode{}
	_ fusefs.HandleReader       = fileNode{}
)

func isSubPath(parent, child string) bool {
	if !strings.HasPrefix(child, parent) {
		return false
	}
	s := child[len(parent):]
	if len(s) == 0 {
		return false
	}
	return s[0] == '/'
}

func (dn dirNode) ReadDirAll(ctx context.Context) (des []fuse.Dirent, err error) {
	names := map[string]bool{}
	for _, fi := range dn.metadata.Files {
		if !isSubPath(dn.path, strings.Join(fi.Path, "/")) {
			continue
		}
		name := fi.Path[len(dn.path)]
		if names[name] {
			continue
		}
		names[name] = true
		de := fuse.Dirent{
			Name: name,
		}
		if len(fi.Path) == len(dn.path)+1 {
			de.Type = fuse.DT_File
		} else {
			de.Type = fuse.DT_Dir
		}
		des = append(des, de)
	}
	return
}

func (dn dirNode) Lookup(ctx context.Context, name string) (_node fusefs.Node, err error) {
	var torrentOffset int64
	for _, fi := range dn.metadata.Files {
		if !isSubPath(dn.path, strings.Join(fi.Path, "/")) {
			torrentOffset += fi.Length
			continue
		}
		if fi.Path[len(dn.path)] != name {
			torrentOffset += fi.Length
			continue
		}
		__node := dn.node
		__node.path = path.Join(__node.path, name)
		if len(fi.Path) == len(dn.path)+1 {
			_node = fileNode{
				node:          __node,
				size:          uint64(fi.Length),
				TorrentOffset: torrentOffset,
			}
		} else {
			_node = dirNode{__node}
		}
		break
	}
	if _node == nil {
		err = fuse.ENOENT
	}
	return
}

func (dn dirNode) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir | defaultMode
	return nil
}

func (me rootNode) Lookup(ctx context.Context, name string) (_node fusefs.Node, err error) {
	for _, t := range me.fs.Client.Torrents() {
		info := t.Info()
		if t.Name() != name || info == nil {
			continue
		}
		__node := node{
			metadata: info,
			FS:       me.fs,
			t:        t,
		}
		if !info.IsDir() {
			_node = fileNode{__node, uint64(info.Length), 0}
		} else {
			_node = dirNode{__node}
		}
		break
	}
	if _node == nil {
		err = fuse.ENOENT
	}
	return
}

func (me rootNode) ReadDirAll(ctx context.Context) (dirents []fuse.Dirent, err error) {
	for _, t := range me.fs.Client.Torrents() {
		info := t.Info()
		if info == nil {
			continue
		}
		dirents = append(dirents, fuse.Dirent{
			Name: info.Name,
			Type: func() fuse.DirentType {
				if !info.IsDir() {
					return fuse.DT_File
				} else {
					return fuse.DT_Dir
				}
			}(),
		})
	}
	return
}

func (rootNode) Attr(ctx context.Context, attr *fuse.Attr) error {
	attr.Mode = os.ModeDir
	return nil
}

// TODO(anacrolix): Why should rootNode implement this?
func (me rootNode) Forget() {
	me.fs.Destroy()
}

func (tfs *TorrentFS) Root() (fusefs.Node, error) {
	return rootNode{tfs}, nil
}

func (me *TorrentFS) Destroy() {
	me.mu.Lock()
	select {
	case <-me.destroyed:
	default:
		close(me.destroyed)
	}
	me.mu.Unlock()
}

func New(cl *torrent.Client) *TorrentFS {
	fs := &TorrentFS{
		Client:    cl,
		destroyed: make(chan struct{}),
	}
	fs.event.L = &fs.mu
	return fs
}

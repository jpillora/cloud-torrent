package dropbox

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jpillora/cloud-torrent/cloudtorrent/fs"
	"github.com/spf13/afero"
	dropbox "github.com/tj/go-dropbox"
)

func New() fs.FS {
	return &dropboxFS{
		root: newFolder("root"),
	}
}

type dropboxFS struct {
	client *dropbox.Client
	config struct {
		Token string
		Base  string
	}
	root *file
}

func (d *dropboxFS) Name() string {
	return "Dropbox"
}

func (d *dropboxFS) Mode() fs.FSMode {
	return fs.RW
}

func (d *dropboxFS) Configure(raw json.RawMessage) (interface{}, error) {
	if err := json.Unmarshal(raw, &d.config); err != nil {
		return nil, err
	}
	if d.config.Token == "" {
		d.client = nil
		return nil, errors.New("API token missing")
	}
	if d.config.Base == "" {
		d.config.Base = "/"
	}
	d.client = dropbox.New(dropbox.NewConfig(d.config.Token))
	return &d.config, nil
}

func (d *dropboxFS) Update(updates chan fs.Node) error {
	c := d.client
	if c == nil {
		return errors.New("API token was removed")
	}
	emit := true
	//list all files in base
	resp, err := c.Files.ListFolder(&dropbox.ListFolderInput{
		Path:      d.config.Base,
		Recursive: true,
	})
	if err != nil {
		return err
	}
	for {
		//move results into memory
		for _, m := range resp.Entries {
			if ch, err := d.updateFile(m); err != nil {
				return err
			} else if ch {
				emit = true
			}
		}
		//emit updates
		if !resp.HasMore && emit {
			updates <- d.root
			emit = false
		}
		//poll next set
		resp, err = c.Files.ListFolderContinue(&dropbox.ListFolderContinueInput{
			Cursor: resp.Cursor,
		})
		if err != nil {
			return err
		}
		if !resp.HasMore {
			time.Sleep(3 * time.Second)
		}
	}
}

func (d *dropboxFS) updateFile(m *dropbox.Metadata) (bool, error) {
	changed := false
	//
	parents := strings.Split(strings.TrimPrefix(m.PathDisplay, "/"), "/")
	if len(parents) == 0 {
		return false, errors.New("no path")
	}
	f := d.root
	name := parents[0]
	//find/initialise all parent nodes
	parents = parents[1:]
	for _, pname := range parents {
		p, ok := f.children[pname]
		if ok {
			//update
		} else if !ok {
			//create missing parent
			p = newFolder(pname)
			f.children[pname] = p
			changed = true
		}
		f = p
	}
	parent := f
	//action
	remove := m.Tag == "deleted"
	add := !remove
	//create/update node
	n, ok := parent.children[name]
	if ok {
		//existing + delete
		if remove {
			delete(parent.children, name)
			changed = true
		}
		//existing + update
		if add && n.update(m) {
			changed = true
		}
	} else if add {
		//new + add
		n = newFile(m)
		parent.children[name] = n
		changed = true
	}
	return changed, nil
}

func (d *dropboxFS) Create(name string) (afero.File, error) {
	return &file{}, nil
}

func (d *dropboxFS) Open(name string) (afero.File, error) {
	return &file{}, nil
}

func (d *dropboxFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return d.Open(name)
}

func (d *dropboxFS) Mkdir(name string, perm os.FileMode) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) MkdirAll(path string, perm os.FileMode) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) Remove(name string) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) RemoveAll(path string) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) Rename(oldname, newname string) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) Stat(name string) (os.FileInfo, error) {
	return nil, errors.New("not supported yet")
}

func (d *dropboxFS) Chmod(name string, mode os.FileMode) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return errors.New("not supported yet")
}

func logf(format string, args ...interface{}) {
	log.Printf("[Dropbox] "+format, args...)
}

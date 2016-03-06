package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

type AferoConfig struct {
	FileLimit int    `json:"fileLimit"`
	BasePath  string `json:"basePath"`
}

//newAferoFs creates a storage.Fs using an afero.Fs
func newAferoFs(afs afero.Fs) Fs {
	f := &fs{afs: afs}
	f.config.FileLimit = 1000
	return f
}

//fs is an implementation of Fs
type fs struct {
	config AferoConfig
	afs    afero.Fs
}

//List is a custom directory walk against the specified filesystem
func (f *fs) Configure(config interface{}) error {
	return nil
}

//List is a custom directory walk against the specified filesystem
func (f *fs) List(basePath string) (*Node, error) {
	info, err := f.afs.Stat(basePath)
	if err != nil {
		return nil, err
	}
	rootNode := &Node{}
	if err := f.listAccumulator(basePath, info, rootNode, new(int)); err != nil {
		return nil, err
	}
	return rootNode, nil
}

func (f *fs) listAccumulator(path string, info os.FileInfo, node *Node, n *int) error {
	if (!info.IsDir() && !info.Mode().IsRegular()) || strings.HasPrefix(info.Name(), ".") {
		return errors.New("Non-regular file")
	}
	(*n)++
	if (*n) > f.config.FileLimit {
		return errors.New("Over file limit") //limit number of files walked
	}
	//set node
	node.Name = info.Name()
	node.Size = info.Size()
	node.Modified = info.ModTime()
	if !info.IsDir() {
		return nil
	}
	//if directory, list children...
	children, err := afero.ReadDir(f.afs, path)
	if err != nil {
		return fmt.Errorf("Failed to list files")
	}
	node.Size = 0
	for _, i := range children {
		c := &Node{}
		p := filepath.Join(path, i.Name())
		if err := f.listAccumulator(p, i, c, n); err != nil {
			continue
		}
		node.Size += c.Size
		node.Children = append(node.Children, c)
	}
	return nil
}

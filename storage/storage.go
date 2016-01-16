package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
)

type Configs struct {
	Disk *DiskConfig `json:"disk"`
}

var DefaultFileLimit = 1000

type Storage struct {
	FileLimit   int
	filesystems map[string]afero.Fs
}

func New() *Storage {
	s := &Storage{
		FileLimit:   DefaultFileLimit,
		filesystems: map[string]afero.Fs{},
	}
	return s
}

func (s *Storage) Configure(c Configs) error {
	var lastErr error
	if c.Disk != nil {
		fs, err := NewDisk(*c.Disk)
		if err == nil {
			s.filesystems["disk"] = fs
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return nil
}

func (s *Storage) FSNames() []string {
	names := make([]string, len(s.filesystems))
	for name, _ := range s.filesystems {
		names = append(names, name)
	}
	return names
}

type Node struct {
	Name     string
	Size     int64
	Modified time.Time
	Children []*Node
}

//List is a custom directory walk against the specified filesystem
func (s *Storage) List(name string) (*Node, error) {
	fs, ok := s.filesystems[name]
	if !ok {
		return nil, errors.New("Missing filesystem")
	}
	rootPath := ""
	info, err := fs.Stat(rootPath)
	if err != nil {
		return nil, err
	}
	rootDir := &Node{}
	if err := s.list(fs, rootPath, info, rootDir, new(int)); err != nil {
		return nil, err
	}
	return rootDir, nil
}

func (s *Storage) list(fs afero.Fs, path string, info os.FileInfo, node *Node, n *int) error {
	if (!info.IsDir() && !info.Mode().IsRegular()) || strings.HasPrefix(info.Name(), ".") {
		return errors.New("Non-regular file")
	}
	(*n)++
	if (*n) > s.FileLimit {
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
	children, err := afero.ReadDir(fs, path)
	if err != nil {
		return fmt.Errorf("Failed to list files")
	}
	node.Size = 0
	for _, i := range children {
		c := &Node{}
		p := filepath.Join(path, i.Name())
		if err := s.list(fs, p, i, c, n); err != nil {
			continue
		}
		node.Size += c.Size
		node.Children = append(node.Children, c)
	}
	return nil
}

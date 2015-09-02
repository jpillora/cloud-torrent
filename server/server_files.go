package server

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type fsNode struct {
	Name     string
	Size     int64
	Modified time.Time
	Children []*fsNode
}

func (s *Server) listFiles() *fsNode {
	rootDir := s.state.Config.DownloadDirectory
	root := &fsNode{}
	if info, err := os.Stat(rootDir); err == nil {
		if err := list(rootDir, info, root); err != nil {
			log.Printf("File listing failed: %s", err)
		}
	}
	return root
}

func (s *Server) serveFiles(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/download/") {
		//dldir is absolute
		dldir := s.state.Config.DownloadDirectory
		file := filepath.Join(dldir, strings.TrimPrefix(r.URL.Path, "/download/"))
		//only allow fetches/deletes inside the dl dir
		if !strings.HasPrefix(file, dldir) || dldir == file {
			http.Error(w, "Nice try\n"+dldir+"\n"+file, http.StatusBadRequest)
			return
		}
		switch r.Method {
		case "GET":
			http.ServeFile(w, r, file)
		case "DELETE":
			if err := os.RemoveAll(file); err != nil {
				http.Error(w, "Delete failed: "+err.Error(), http.StatusInternalServerError)
			}
		default:
			http.Error(w, "Not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	s.static.ServeHTTP(w, r)
}

//custom directory walk

func list(path string, info os.FileInfo, node *fsNode) error {
	if (!info.IsDir() && !info.Mode().IsRegular()) || strings.HasPrefix(info.Name(), ".") {
		return fmt.Errorf("Non-regular file")
	}
	node.Name = info.Name()
	node.Size = info.Size()
	node.Modified = info.ModTime()
	if !info.IsDir() {
		return nil
	}
	children, err := ioutil.ReadDir(path)
	if err != nil {
		return fmt.Errorf("Failed to list files")
	}
	node.Size = 0
	for _, i := range children {
		c := &fsNode{}
		p := filepath.Join(path, i.Name())
		if err := list(p, i, c); err != nil {
			continue
		}
		node.Size += c.Size
		node.Children = append(node.Children, c)
	}
	return nil
}

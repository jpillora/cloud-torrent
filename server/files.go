package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jpillora/archive"
)

const fileNumberLimit = 1000

// FsNode is a node in the downloads file tree, synced to the browser. A
// directory node has Children; a file node does not.
type FsNode struct {
	Name     string
	Size     int64
	Modified time.Time
	Children []*FsNode
}

// listFiles walks rootDir and returns it as the root FsNode. Dotfiles and
// non-regular files are skipped, and the walk is capped at fileNumberLimit
// entries. A missing rootDir yields an empty node.
func listFiles(rootDir string) *FsNode {
	root := &FsNode{}
	if info, err := os.Stat(rootDir); err == nil {
		n := 0
		_ = list(rootDir, info, root, &n)
	}
	return root
}

var (
	errNotRegular = errors.New("non-regular file")
	errFileLimit  = errors.New("over file limit")
)

func list(path string, info os.FileInfo, node *FsNode, n *int) error {
	if (!info.IsDir() && !info.Mode().IsRegular()) || strings.HasPrefix(info.Name(), ".") {
		return errNotRegular
	}
	*n++
	if *n > fileNumberLimit {
		return errFileLimit
	}
	node.Name = info.Name()
	node.Size = info.Size()
	node.Modified = info.ModTime()
	if !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	node.Size = 0
	for _, e := range entries {
		ci, err := e.Info()
		if err != nil {
			continue
		}
		child := &FsNode{}
		if err := list(filepath.Join(path, e.Name()), ci, child, n); err != nil {
			continue
		}
		node.Size += child.Size
		node.Children = append(node.Children, child)
	}
	return nil
}

// downloadHandler serves and deletes files under the directory returned by dir
// (read per-request, so it tracks config changes). Mount it stripped of its URL
// prefix:
//
//	http.StripPrefix("/download/", downloadHandler(dir, onChange))
//
// GET streams a file, or a zip of a directory; DELETE removes the target. Paths
// that escape the download directory (or target the root itself) are rejected.
// onChange (if non-nil) is invoked after a successful DELETE so the file tree
// can be refreshed immediately rather than on the next poll.
func downloadHandler(dir func() string, onChange func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		full, ok := safeJoin(dir(), r.URL.Path)
		if !ok {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		info, err := os.Stat(full)
		if err != nil {
			http.Error(w, "file stat error: "+err.Error(), http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if info.IsDir() {
				w.Header().Set("Content-Type", "application/zip")
				w.Header().Set("Content-Disposition", "attachment; filename=\""+info.Name()+".zip\"")
				a := archive.NewZipWriter(w)
				a.AddDir(full)
				a.Close()
				return
			}
			f, err := os.Open(full)
			if err != nil {
				http.Error(w, "file open error: "+err.Error(), http.StatusBadRequest)
				return
			}
			defer f.Close()
			http.ServeContent(w, r, info.Name(), info.ModTime(), f)
		case http.MethodDelete:
			if err := os.RemoveAll(full); err != nil {
				http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// reflect the removal immediately instead of waiting for the poll
			if onChange != nil {
				onChange()
			}
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// safeJoin joins rel onto root, rejecting paths that escape root or target the
// root itself. root is cleaned; rel is treated as a slash-separated URL path.
func safeJoin(root, rel string) (string, bool) {
	root = filepath.Clean(root)
	full := filepath.Join(root, filepath.FromSlash(rel))
	if full == root || !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

package server

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		// Root resolves links during each filesystem operation and prevents
		// escapes even if a path component changes concurrently.
		name := filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/"))
		if !filepath.IsLocal(name) || filepath.Clean(name) == "." {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		root, err := os.OpenRoot(dir())
		if err != nil {
			http.Error(w, "download directory unavailable", http.StatusBadRequest)
			return
		}
		defer root.Close()
		f, err := root.Open(name)
		if err != nil {
			http.Error(w, "file open error: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			http.Error(w, "file stat error: "+err.Error(), http.StatusBadRequest)
			return
		}
		rootInfo, err := root.Stat(".")
		if err != nil || os.SameFile(rootInfo, info) {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if info.IsDir() {
				folder, err := root.OpenRoot(name)
				if err != nil {
					http.Error(w, "directory open error: "+err.Error(), http.StatusBadRequest)
					return
				}
				defer folder.Close()
				w.Header().Set("Content-Type", "application/zip")
				w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name() + ".zip"}))
				if err := writeDownloadZip(w, folder); err != nil {
					log.Printf("download archive: %v", err)
				}
				return
			}
			if !info.Mode().IsRegular() {
				http.Error(w, "not a regular file", http.StatusBadRequest)
				return
			}
			http.ServeContent(w, r, info.Name(), info.ModTime(), f)
		case http.MethodDelete:
			// Close the read handle before unlinking (required on Windows).
			f.Close()
			// RemoveAll unlinks a final symlink, never its target.
			if err := root.RemoveAll(name); err != nil {
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

// writeDownloadZip skips links and special files, matching the downloads tree.
// Every archive read remains confined even if an entry is replaced mid-walk.
func writeDownloadZip(w io.Writer, root *os.Root) error {
	zw := zip.NewWriter(w)
	walkErr := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		header.Method = zip.Store
		dst, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(dst, f)
		return err
	})
	return errors.Join(walkErr, zw.Close())
}

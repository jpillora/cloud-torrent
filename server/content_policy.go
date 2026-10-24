package server

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// contentPolicy protects operational files by path and identity. Identity checks
// cover hard links as well as renamed/symlinked aliases of configured files.
type contentPolicy struct {
	paths []string
	infos []os.FileInfo
}

func newContentPolicy(paths ...string) *contentPolicy {
	p := &contentPolicy{}
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		p.paths = append(p.paths, filepath.Clean(abs))
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			p.paths = append(p.paths, resolved)
		}
		if info, err := os.Stat(abs); err == nil {
			p.infos = append(p.infos, info)
		}
	}
	return p
}

func (p *contentPolicy) hidden(path string, info os.FileInfo) bool {
	if info != nil && !info.IsDir() && (info.Name() == "cloud-torrent.json" || strings.HasPrefix(info.Name(), ".cloud-torrent-config-")) {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	resolved, _ := filepath.EvalSymlinks(abs)
	for _, protected := range p.paths {
		if (resolved != "" && pathWithin(protected, resolved)) || pathWithin(protected, abs) {
			return true
		}
	}
	if info != nil {
		for _, protected := range p.infos {
			if os.SameFile(info, protected) {
				return true
			}
		}
	}
	return false
}

// Deleting a directory must also protect nested state and hard-linked aliases.
// Bound the check and fail closed rather than allowing a partial inspection.
func (p *contentPolicy) checkDelete(root *os.Root, name string) error {
	initial, err := root.Lstat(name)
	if err != nil {
		return err
	}
	// RemoveAll unlinks a final symlink; inspecting its target subtree would
	// reject harmless link deletion and follow paths removal never visits.
	if initial.Mode()&os.ModeSymlink != 0 {
		if p.hidden(filepath.Join(root.Name(), name), initial) {
			return fmt.Errorf("protected application state")
		}
		return nil
	}
	visited := 0
	return fs.WalkDir(root.FS(), name, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		visited++
		if visited > 10000 {
			return fmt.Errorf("directory too large to safely inspect for control files")
		}
		info, err := root.Lstat(path)
		if err != nil {
			return err
		}
		if p.hidden(filepath.Join(root.Name(), path), info) {
			return fmt.Errorf("protected application state")
		}
		return nil
	})
}

func (s *Server) contentPaths() []string {
	return []string{s.ConfigPath, s.StateDirectory, s.KeyPath, s.CertPath, s.WatchDirectory, s.SearchConfigPath}
}

func validateStateDirectory(download, state string) error {
	if state == "" {
		return nil
	}
	// Resolve existing ancestors too: a not-yet-created state directory may have
	// a symlinked parent. Reject overlap in either direction.
	canonical := func(path string) (string, error) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		suffix := ""
		for {
			resolved, err := filepath.EvalSymlinks(abs)
			if err == nil {
				return filepath.Join(resolved, suffix), nil
			}
			if !os.IsNotExist(err) {
				return "", err
			}
			parent := filepath.Dir(abs)
			if parent == abs {
				return "", err
			}
			suffix = filepath.Join(filepath.Base(abs), suffix)
			abs = parent
		}
	}
	a, err := canonical(download)
	if err != nil {
		return err
	}
	b, err := canonical(state)
	if err != nil {
		return err
	}
	if pathWithin(a, b) || pathWithin(b, a) {
		return fmt.Errorf("state directory and download directory must not overlap")
	}
	return nil
}

func pathWithin(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && filepath.IsLocal(rel)
}

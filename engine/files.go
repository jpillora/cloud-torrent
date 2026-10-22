package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrActiveDownload = errors.New("stop the torrent before deleting active downloads")

// RemoveDownload serializes deletion with torrent starts and reconfiguration.
// The opened root must still be the configured directory. Files and every
// ancestor of an active incomplete file are protected, including path aliases.
func (e *Engine) RemoveDownload(root *os.Root, name string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	if !filepath.IsLocal(name) || filepath.Clean(name) == "." {
		return fmt.Errorf("invalid path")
	}
	current, err := os.Stat(e.config.DownloadDirectory)
	if err != nil {
		return err
	}
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(current, opened) {
		return fmt.Errorf("download directory changed; retry deletion")
	}
	target, err := root.Stat(name)
	if err != nil {
		return err
	}
	if os.SameFile(opened, target) {
		return fmt.Errorf("invalid path")
	}
	if e.client != nil {
		e.refreshLocked()
	}
	clean := filepath.Clean(name)
	for _, t := range e.ts {
		if !t.Started {
			continue
		}
		for _, f := range t.Files {
			if f == nil || !f.Started || f.Percent >= 100 {
				continue
			}
			path := filepath.Clean(filepath.FromSlash(f.Path))
			for _, active := range []string{path, path + ".part"} {
				if active == clean || strings.HasPrefix(active, clean+string(filepath.Separator)) {
					return ErrActiveDownload
				}
				// Resolve aliases by comparing the target with each active ancestor.
				for p := active; p != "."; p = filepath.Dir(p) {
					if info, err := root.Stat(p); err == nil && os.SameFile(info, target) {
						return ErrActiveDownload
					}
				}
			}
		}
	}
	return root.RemoveAll(name)
}

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
	var affected []*Torrent
	for _, t := range e.ts {
		for _, f := range t.Files {
			if f == nil || !downloadContains(root, target, clean, f.Path) {
				continue
			}
			if t.Started && f.Started && f.Percent < 100 {
				return ErrActiveDownload
			}
			// Keep scanning: another file in this torrent may still be active.
			if len(affected) == 0 || affected[len(affected)-1] != t {
				affected = append(affected, t)
			}
		}
	}
	// Close affected handles before unlinking so in-flight writes cannot use
	// deleted files. Reopening storage discards cached completion/chunk state.
	saved := make([]savedTorrent, 0, len(affected))
	for _, t := range affected {
		saved = append(saved, saveTorrent(t))
		t.t.Drop()
		delete(e.ts, t.InfoHash)
	}
	err = root.RemoveAll(name)
	// Restore selection policy even if deletion failed. Unrelated handles stay live.
	for _, state := range saved {
		model, restoreErr := restoreTorrent(e.client, state)
		if restoreErr != nil {
			err = errors.Join(err, restoreErr)
			continue
		}
		e.ts[model.InfoHash] = model
		e.resumeTorrentLocked(model)
	}
	return err
}

// Match storage names, parent folders and aliases just as the deletion guard does.
func downloadContains(root *os.Root, target os.FileInfo, clean, filePath string) bool {
	path := filepath.Clean(filepath.FromSlash(filePath))
	for _, stored := range []string{path, path + ".part"} {
		if stored == clean || strings.HasPrefix(stored, clean+string(filepath.Separator)) {
			return true
		}
		for p := stored; p != "."; p = filepath.Dir(p) {
			if info, err := root.Stat(p); err == nil && os.SameFile(info, target) {
				return true
			}
		}
	}
	return false
}

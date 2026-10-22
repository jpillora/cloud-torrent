package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jpillora/cloud-torrent/engine"
)

func readConfig(path string, defaults engine.Config) (engine.Config, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaults, nil
	}
	if err != nil {
		return defaults, fmt.Errorf("read configuration %q: %w", path, err)
	}
	if err := json.Unmarshal(b, &defaults); err != nil {
		return defaults, fmt.Errorf("malformed configuration %q: %w", path, err)
	}
	return defaults, nil
}

type pendingConfig string

// Stage in the destination directory so commit is a single atomic rename.
// All fallible writing happens before the live engine is reconfigured.
func stageConfig(path string, c engine.Config) (pendingConfig, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode configuration: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".cloud-torrent-config-*")
	if err != nil {
		return "", fmt.Errorf("stage configuration %q: %w", path, err)
	}
	pending := pendingConfig(f.Name())
	ok := false
	defer func() {
		f.Close()
		if !ok {
			pending.discard()
		}
	}()
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		mode = info.Mode().Perm()
	}
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("write configuration %q: %w", path, err)
	}
	ok = true
	return pending, nil
}

func (p pendingConfig) discard() { _ = os.Remove(string(p)) }

func (p pendingConfig) commit(path string) error {
	if err := os.Rename(string(p), path); err != nil {
		return fmt.Errorf("replace configuration %q: %w", path, err)
	}
	return nil
}

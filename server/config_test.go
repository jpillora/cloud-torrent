package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpillora/cloud-torrent/engine"
	"github.com/jpillora/velox"
)

func TestReadConfigErrors(t *testing.T) {
	dir := t.TempDir()
	defaults := engine.Config{AutoStart: true}
	if got, err := readConfig(filepath.Join(dir, "missing.json"), defaults); err != nil || got != defaults {
		t.Fatalf("missing config should use defaults: %+v %v", got, err)
	}
	if _, err := readConfig(dir, defaults); err == nil || !strings.Contains(err.Error(), "read configuration") {
		t.Fatalf("directory read should fail: %v", err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfig(bad, defaults); err == nil || !strings.Contains(err.Error(), "malformed configuration") {
		t.Fatalf("malformed config should fail: %v", err)
	}
}

func TestConfigurationPersistenceAndRollback(t *testing.T) {
	dir := t.TempDir()
	s := &Server{engine: engine.New(), ConfigPath: filepath.Join(dir, "config.json")}
	t.Cleanup(func() { s.engine.Close() })
	_ = velox.SyncHandler(&s.state)
	original := engine.Config{DownloadDirectory: t.TempDir(), IncomingPort: freePort(t)}
	if err := s.reconfigure(original); err != nil {
		t.Fatal(err)
	}
	originalBytes, err := os.ReadFile(s.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.AutoStart = true
	assertRejected := func(path, message string) {
		t.Helper()
		s.ConfigPath = path
		body, _ := json.Marshal(changed)
		rr := httptest.NewRecorder()
		s.handleAPI(rr, httptest.NewRequest(http.MethodPost, "/api/configure", strings.NewReader(string(body))))
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), message) {
			t.Fatalf("failed persistence returned %d: %s", rr.Code, rr.Body.String())
		}
		if s.engine.Config() != original || s.state.Config != original {
			t.Fatalf("failed save changed configuration: engine=%+v state=%+v", s.engine.Config(), s.state.Config)
		}
	}
	assertRejected(filepath.Join(dir, "missing", "config.json"), "stage configuration")
	// Rename over a directory fails after application, exercising rollback.
	assertRejected(dir, "replace configuration")
	s.ConfigPath = filepath.Join(dir, "config.json")
	if b, err := os.ReadFile(s.ConfigPath); err != nil || string(b) != string(originalBytes) {
		t.Fatalf("failed save modified existing file: %s %v", b, err)
	}
	if err := os.Chmod(s.ConfigPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.reconfigure(changed); err != nil {
		t.Fatal(err)
	}
	loaded, err := readConfig(s.ConfigPath, engine.Config{})
	if err != nil || loaded != changed || s.engine.Config() != changed || s.state.Config != changed {
		t.Fatalf("successful save did not persist/apply: %+v %v", loaded, err)
	}
	info, err := os.Stat(s.ConfigPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("save changed config permissions: %v", err)
	}
	if files, err := filepath.Glob(filepath.Join(dir, ".cloud-torrent-config-*")); err != nil || len(files) != 0 {
		t.Fatalf("staged files leaked: %v %v", files, err)
	}
}

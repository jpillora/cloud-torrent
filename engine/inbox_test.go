package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInboxReceiptCapacityAndValidation(t *testing.T) {
	e := isolatedBandwidthEngine(t, t.TempDir(), 0, 0, false, t.TempDir())
	_, specs := queueSpecs(t)
	if err := e.ImportTorrent(specs[0], "bad"); err == nil {
		t.Fatal("invalid digest accepted")
	}
	receipt := strings.Repeat("AB", 32)
	bandwidthMust(t, e.ImportTorrent(specs[0], receipt))
	if !e.HasInboxReceipt(strings.ToLower(receipt)) {
		t.Fatal("receipt not canonicalized")
	}
	e.mut.Lock()
	for i := 0; len(e.inboxReceipts) < maxInboxReceipts; i++ {
		e.inboxReceipts[fmt.Sprintf("%064x", i)] = true
	}
	e.mut.Unlock()
	if err := e.ImportTorrent(specs[1], strings.Repeat("cd", 32)); err != ErrInboxReceiptsFull {
		t.Fatalf("receipt cap: %v", err)
	}
	if n, r := e.InboxCounts(); n != 1 || r != maxInboxReceipts {
		t.Fatalf("failed capacity changed state: %d %d", n, r)
	}
}
func TestInboxRejectsCorruptReceiptStore(t *testing.T) {
	for _, kind := range []string{"invalid", "duplicate", "noncanonical", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			e := New()
			bandwidthMust(t, e.EnablePersistence(dir))
			e.Close()
			receipts := []string{"bad"}
			switch kind {
			case "duplicate":
				receipts = []string{strings.Repeat("ab", 32), strings.Repeat("ab", 32)}
			case "noncanonical":
				receipts = []string{strings.Repeat("AB", 32)}
			case "oversize":
				receipts = make([]string, maxInboxReceipts+1)
			}
			body, err := json.Marshal(sessionDisk{Version: 1, InboxReceipts: receipts})
			bandwidthMust(t, err)
			bandwidthMust(t, os.WriteFile(filepath.Join(dir, "sessions.json"), body, 0600))
			reopened := New()
			defer reopened.Close()
			if err := reopened.EnablePersistence(dir); err == nil {
				t.Fatal("accepted corrupt store")
			}
		})
	}
}

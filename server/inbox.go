package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/jpillora/cloud-torrent/engine"
)

const maxInboxEntries = 256
const maxInboxImportsPerTick = 2
const inboxSettleTime = time.Second

type InboxError struct{ Name, Message string }
type InboxStatus struct {
	Enabled  bool
	Pending  int
	Accepted int
	Errors   []InboxError
}
type inboxObservation struct {
	info                            os.FileInfo
	seenAt                          time.Time
	processed, failed, queueBlocked bool
	blockedCount                    int
	message                         string
}
type inboxWatcher struct {
	server    *Server
	root      *os.Root
	directory os.FileInfo
	seen      map[string]*inboxObservation
	attempts  int
}

func (s *Server) validateInboxLayout(download string) error {
	if s.WatchDirectory == "" {
		return nil
	}
	if err := validateStateDirectory(download, s.WatchDirectory); err != nil {
		return errors.New("watch directory and downloads must not overlap")
	}
	if s.StateDirectory != "" {
		if err := validateStateDirectory(s.StateDirectory, s.WatchDirectory); err != nil {
			return errors.New("watch directory and private state must not overlap")
		}
	}
	return nil
}
func newInboxWatcher(s *Server) (*inboxWatcher, error) {
	if s.WatchDirectory == "" {
		return nil, nil
	}
	if err := s.validateInboxLayout(s.engine.Config().DownloadDirectory); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.WatchDirectory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(s.WatchDirectory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("watch directory must be a real directory")
	}
	if info.Mode().Perm()&0002 != 0 {
		return nil, errors.New("watch directory must not be writable by others")
	}
	root, err := os.OpenRoot(s.WatchDirectory)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, errors.New("watch directory changed while opening")
	}
	return &inboxWatcher{server: s, root: root, directory: info, seen: map[string]*inboxObservation{}}, nil
}
func (s *Server) startInbox() (func(), error) {
	watcher, err := newInboxWatcher(s)
	if err != nil {
		return nil, err
	}
	if watcher == nil {
		return func() {}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer watcher.root.Close()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			status := watcher.step(time.Now())
			s.state.Lock()
			s.state.Inbox = status
			s.state.Unlock()
			s.state.Push()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); <-done }, nil
}
func sameInboxFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}
func (w *inboxWatcher) step(now time.Time) InboxStatus {
	status := InboxStatus{Enabled: true, Errors: []InboxError{}}
	count, receipts := w.server.engine.InboxCounts()
	status.Accepted = receipts
	current, err := os.Lstat(w.server.WatchDirectory)
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(current, w.directory) || current.Mode().Perm()&0002 != 0 {
		status.Errors = append(status.Errors, InboxError{Message: "Inbox directory changed or is unavailable; restart the watcher."})
		return status
	}
	dir, err := w.root.Open(".")
	if err != nil {
		status.Errors = append(status.Errors, InboxError{Message: "Cannot read inbox directory."})
		return status
	}
	entries, readErr := dir.ReadDir(maxInboxEntries + 1)
	dir.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		status.Errors = append(status.Errors, InboxError{Message: "Cannot read inbox directory."})
		return status
	}
	if len(entries) > maxInboxEntries {
		status.Errors = append(status.Errors, InboxError{Message: "Inbox has more than 256 entries; move old files out before importing."})
		return status
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	present := map[string]bool{}
	attempted := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
			continue
		}
		present[name] = true
		info, err := w.root.Lstat(name)
		if err != nil {
			continue
		}
		observed := w.seen[name]
		if observed == nil || !sameInboxFile(observed.info, info) {
			observed = &inboxObservation{info: info, seenAt: now}
			w.seen[name] = observed
		}
		switch {
		case !info.Mode().IsRegular():
			observed.failed = true
			observed.message = "Only regular .torrent files are accepted; links and directories are rejected."
		case info.Size() <= 0 || info.Size() > maxRequestBytes:
			observed.failed = true
			observed.message = "Torrent file must contain 1 byte to 10 MiB. Change the file to retry."
		case observed.processed:
		case observed.failed:
		case observed.queueBlocked && observed.blockedCount == count:
			status.Pending++
		case now.Sub(observed.seenAt) < inboxSettleTime || attempted >= maxInboxImportsPerTick:
			status.Pending++
		default:
			attempted++
			w.attempts++
			body, err := w.readStableFile(name, observed.info)
			if err != nil {
				observed.failed = true
				observed.message = "File changed or could not be read safely. Touch or rename it to retry."
				break
			}
			digest := sha256.Sum256(body)
			receipt := hex.EncodeToString(digest[:])
			if w.server.engine.HasInboxReceipt(receipt) {
				observed.processed = true
				observed.message = ""
				break
			}
			spec, err := parseTorrentBytes(body)
			if err != nil {
				observed.failed = true
				observed.message = "Invalid torrent metadata. Correct or replace this file to retry."
				break
			}
			if err = w.server.engine.ImportTorrent(spec, receipt); err != nil {
				if errors.Is(err, engine.ErrAdmissionFull) {
					observed.queueBlocked = true
					observed.blockedCount = count
					observed.message = "Torrent queue is full. Remove a torrent to retry automatically."
					status.Pending++
					break
				}
				observed.failed = true
				if errors.Is(err, engine.ErrInboxReceiptsFull) {
					observed.message = "Inbox receipt limit reached (4096 files). Use manual import."
				} else {
					observed.message = "Torrent validation or session save failed. Check outbound grants and state storage, then touch or rename the file to retry."
				}
				break
			}
			observed.processed = true
			observed.queueBlocked = false
			observed.message = ""
			count, receipts = w.server.engine.InboxCounts()
			status.Accepted = receipts
		}
		if observed.message != "" && len(status.Errors) < 8 {
			status.Errors = append(status.Errors, InboxError{Name: name, Message: observed.message})
		}
	}
	for name := range w.seen {
		if !present[name] {
			delete(w.seen, name)
		}
	}
	return status
}
func (w *inboxWatcher) readStableFile(name string, before os.FileInfo) ([]byte, error) {
	f, err := openInboxFile(w.root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !sameInboxFile(before, opened) {
		return nil, errors.New("file replaced")
	}
	body, err := io.ReadAll(io.LimitReader(f, maxRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxRequestBytes {
		return nil, errors.New("file too large")
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	pathAfter, err := w.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !sameInboxFile(before, after) || !sameInboxFile(after, pathAfter) || !pathAfter.Mode().IsRegular() {
		return nil, errors.New("file changed")
	}
	return body, nil
}
func parseTorrentBytes(body []byte) (*torrent.TorrentSpec, error) {
	if len(body) == 0 || len(body) > maxRequestBytes {
		return nil, errors.New("torrent exceeds size limit")
	}
	var info metainfo.MetaInfo
	if err := bencode.Unmarshal(body, &info); err != nil {
		return nil, errors.New("invalid torrent file")
	}
	spec, err := torrent.TorrentSpecFromMetaInfoErr(&info)
	if err != nil {
		return nil, errors.New("invalid torrent metadata")
	}
	return spec, nil
}

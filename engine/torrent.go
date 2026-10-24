package engine

import (
	"time"

	"github.com/anacrolix/torrent"
)

// Torrent is the engine's view-model for a single torrent. It is projected from
// the underlying anacrolix/torrent on every poll and synced to the browser.
type Torrent struct {
	// derived from anacrolix/torrent
	InfoHash   string
	Name       string
	Loaded     bool  // true once metadata (Info) has arrived
	Downloaded int64 // verified content present on disk, including existing files
	Size       int64
	Files      []*File
	// cloud-torrent state
	Uploaded          int64 // durable payload uploads, reused by seeding budgets
	SeedSeconds       int64
	SeedRatio         float64
	StopReason        string
	SeedingOverride   bool
	State             string // queued, metadata, downloading, paused, completed, seeding or stopped-by-policy
	QueuePosition     int    // one-based FIFO waiting position, zero when not queued
	Started           bool
	Dropped           bool
	Percent           float32 // 0..100, truncated to 2 dp
	DownloadRate      float32 // network payload bytes/sec over the sampling interval
	UploadRate        float32 // network payload bytes/sec over the sampling interval
	NetworkDownloaded int64   // payload received during this torrent client-handle session
	UsefulDownloaded  int64   // useful payload received during the same session
	ActivePeers       int     // established peer connections, excludes webseeds
	KnownPeers        int     // upstream known peers, including active/pending connections

	account        seedAccount
	lastUploaded   int64
	accountedAt    time.Time
	wasSeeding     bool
	wantStart      bool
	order          uint64
	metadataPaused bool
	selection      map[string]bool // retained selected files while paused
	autoStart      bool            // pending whole-torrent start, from initial policy or explicit Start
	spec           *torrent.TorrentSpec
	t              *torrent.Torrent
	transferSample transferSample
}

// File is the engine's view-model for one file within a torrent.
type File struct {
	Path      string
	Size      int64
	Chunks    int
	Completed int
	Started   bool
	Selected  bool // durable selection intent, including while paused or queued
	Percent   float32

	f *torrent.File
}

// Update refreshes the view-model from the underlying torrent.
func (t *Torrent) Update(tt *torrent.Torrent) {
	t.Name = tt.Name()
	t.Loaded = tt.Info() != nil
	if t.t != tt {
		t.transferSample = transferSample{}
		t.DownloadRate = 0
		t.UploadRate = 0
	}
	t.updateTransfers(tt.Stats(), time.Now())
	if t.Loaded {
		t.updateLoaded(tt)
	}
	t.t = tt
}

// updateLoaded refreshes the fields that are only available once metadata has
// arrived: size, per-file progress, overall percent and the download rate.
func (t *Torrent) updateLoaded(tt *torrent.Torrent) {
	t.Size = tt.Length()

	tfiles := tt.Files()
	if len(tfiles) > 0 && t.Files == nil {
		t.Files = make([]*File, len(tfiles))
	}
	for i, tf := range tfiles {
		file := t.Files[i]
		if file == nil {
			file = &File{Path: tf.Path()}
			t.Files[i] = file
		}
		chunks := tf.State()
		completed := 0
		for _, p := range chunks {
			if p.Complete {
				completed++
			}
		}
		file.Size = tf.Length()
		file.Chunks = len(chunks)
		file.Completed = completed
		file.Percent = percent(int64(completed), int64(len(chunks)))
		file.f = tf
	}

	done := tt.BytesCompleted()
	t.Percent = percent(done, t.Size)
	t.Downloaded = done
}

// percent returns 100*n/total truncated to two decimal places (0 when total
// is 0), matching the original cloud-torrent behaviour.
func percent(n, total int64) float32 {
	if total <= 0 || n <= 0 {
		return 0
	}
	if n >= total {
		return 100
	}
	return float32(int(float64(10000)*(float64(n)/float64(total)))) / 100
}

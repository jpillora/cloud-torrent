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
	Loaded     bool // true once metadata (Info) has arrived
	Downloaded int64
	Size       int64
	Files      []*File
	// cloud-torrent state
	Uploaded        int64
	SeedSeconds     int64
	SeedRatio       float64
	StopReason      string
	SeedingOverride bool
	State           string // queued, metadata, downloading, paused, completed or seeding
	QueuePosition   int    // one-based FIFO waiting position, zero when not queued
	Started         bool
	Dropped         bool
	Percent         float32 // 0..100, truncated to 2 dp
	DownloadRate    float32 // bytes/sec since the last update

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
	updatedAt      time.Time
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

	now := time.Now()
	done := tt.BytesCompleted()
	t.Percent = percent(done, t.Size)
	if !t.updatedAt.IsZero() {
		dt := float32(now.Sub(t.updatedAt))
		db := float32(done - t.Downloaded)
		if rate := db * (float32(time.Second) / dt); rate >= 0 {
			t.DownloadRate = rate
		}
	}
	t.Downloaded = done
	t.updatedAt = now
}

// percent returns 100*n/total truncated to two decimal places (0 when total
// is 0), matching the original cloud-torrent behaviour.
func percent(n, total int64) float32 {
	if total == 0 {
		return 0
	}
	return float32(int(float64(10000)*(float64(n)/float64(total)))) / 100
}

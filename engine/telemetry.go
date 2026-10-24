package engine

import (
	"math"
	"time"

	"github.com/anacrolix/torrent"
)

const transferSampleInterval = 250 * time.Millisecond

type transferSample struct {
	at                 time.Time
	received, uploaded int64
}

func safeTransferCount(count int64) int64 {
	return min(max(count, 0), maxSafeTransferBytes)
}
func transferRate(delta int64, elapsed time.Duration) float32 {
	if delta <= 0 || elapsed <= 0 {
		return 0
	}
	rate := float64(delta) / elapsed.Seconds()
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
		return 0
	}
	return float32(min(rate, float64(maxSafeTransferBytes)))
}
func (t *Torrent) clearTransfers() {
	t.transferSample = transferSample{}
	t.DownloadRate = 0
	t.UploadRate = 0
	t.NetworkDownloaded = 0
	t.UsefulDownloaded = 0
	t.ActivePeers = 0
	t.KnownPeers = 0
}
func (t *Torrent) updateTransfers(stats torrent.TorrentStats, now time.Time) {
	t.NetworkDownloaded = safeTransferCount(stats.BytesReadData.Int64())
	t.UsefulDownloaded = min(safeTransferCount(stats.BytesReadUsefulData.Int64()), t.NetworkDownloaded)
	t.ActivePeers = max(stats.ActivePeers, 0)
	t.KnownPeers = max(stats.TotalPeers, t.ActivePeers)
	uploaded := safeTransferCount(stats.BytesWrittenData.Int64())
	sample := &t.transferSample
	if sample.at.IsZero() || !now.After(sample.at) || t.NetworkDownloaded < sample.received || uploaded < sample.uploaded {
		*sample = transferSample{now, t.NetworkDownloaded, uploaded}
		t.DownloadRate = 0
		t.UploadRate = 0
		return
	}
	elapsed := now.Sub(sample.at)
	// Update can run several times during a single scheduler pass. Sampling less
	// often prevents those duplicate projections from erasing a useful rate.
	if elapsed < transferSampleInterval {
		return
	}
	t.DownloadRate = transferRate(t.NetworkDownloaded-sample.received, elapsed)
	t.UploadRate = transferRate(uploaded-sample.uploaded, elapsed)
	*sample = transferSample{now, t.NetworkDownloaded, uploaded}
}

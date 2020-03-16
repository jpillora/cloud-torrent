package engine

import (
	"time"

	"github.com/anacrolix/torrent"
)

type Torrent struct {
	// put at first postition to prevent memorty align issues.
	Stats torrent.TorrentStats

	//anacrolix/torrent
	InfoHash   string
	Name       string
	Magnet     string
	Loaded     bool
	Downloaded int64
	Uploaded   int64
	Size       int64
	Files      []*File

	//cloud torrent
	Started       bool
	Done          bool
	DoneCmdCalled bool
	IsDoneReady   bool
	Percent       float32
	DownloadRate  float32
	UploadRate    float32
	SeedRatio     float32
	AddedAt       time.Time
	StartedAt     time.Time
	t             *torrent.Torrent
	dropWait      chan struct{}
	updatedAt     time.Time
}

type File struct {
	//anacrolix/torrent
	Path          string
	Size          int64
	Completed     int64
	Done          bool
	DoneCmdCalled bool
	//cloud torrent
	Started bool
	Percent float32
	f       *torrent.File
}

// Update retrive info from torrent.Torrent
func (torrent *Torrent) Update(t *torrent.Torrent) {
	torrent.Name = t.Name()
	if t.Info() != nil {
		torrent.Loaded = true
		torrent.updateLoaded(t)
	}
	if torrent.Magnet == "" {
		meta := t.Metainfo()
		m := meta.Magnet(t.Name(), t.InfoHash())
		torrent.Magnet = m.String()
	}
	torrent.t = t
}

func (torrent *Torrent) updateLoaded(t *torrent.Torrent) {

	torrent.Size = t.Length()
	tfiles := t.Files()
	if len(tfiles) > 0 && torrent.Files == nil {
		torrent.Files = make([]*File, len(tfiles))
	}
	//merge in files
	for i, f := range tfiles {
		path := f.Path()
		file := torrent.Files[i]
		if file == nil {
			file = &File{Path: path, Started: torrent.Started}
			torrent.Files[i] = file
		}

		file.Size = f.Length()
		file.Completed = f.BytesCompleted()
		file.Percent = percent(file.Completed, file.Size)
		file.Done = (file.Completed == file.Size)
		file.f = f
	}

	torrent.Stats = t.Stats()
	now := time.Now()
	bytes := t.BytesCompleted()
	ulbytes := torrent.Stats.BytesWrittenData.Int64()

	// calculate rate
	if !torrent.updatedAt.IsZero() {
		dtinv := float32(time.Second) / float32(now.Sub(torrent.updatedAt))

		dldb := float32(bytes - torrent.Downloaded)
		torrent.DownloadRate = dldb * dtinv

		uldb := float32(ulbytes - torrent.Uploaded)
		torrent.UploadRate = uldb * dtinv
	}

	torrent.Downloaded = bytes
	torrent.Uploaded = ulbytes

	torrent.updatedAt = now
	torrent.Percent = percent(bytes, torrent.Size)
	torrent.Done = t.BytesMissing() == 0

	// calculate ratio
	bRead := torrent.Stats.BytesReadData.Int64()
	bWrite := torrent.Stats.BytesWritten.Int64()
	if bRead > 0 {
		torrent.SeedRatio = float32(bWrite) / float32(bRead)
	}
}

func percent(n, total int64) float32 {
	if total == 0 {
		return float32(0)
	}
	return float32(int(float64(10000)*(float64(n)/float64(total)))) / 100
}

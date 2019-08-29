package engine

import (
	"time"

	"github.com/anacrolix/torrent"
)

type Torrent struct {
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
	Percent       float32
	DownloadRate  float32
	UploadRate    float32
	SeedRatio     float32
	AddedAt       time.Time
	Stats         torrent.TorrentStats
	t             *torrent.Torrent
	updatedAt     time.Time
}

type File struct {
	//anacrolix/torrent
	Path          string
	Size          int64
	Chunks        int
	Completed     int
	Done          bool
	DoneCmdCalled bool
	//cloud torrent
	Started bool
	Percent float32
	f       torrent.File
}

// Update retrive info from torrent.Torrent
func (torrent *Torrent) Update(t *torrent.Torrent) {
	torrent.Name = t.Name()
	torrent.Loaded = t.Info() != nil
	if torrent.Loaded {
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
	totalChunks := 0
	totalCompleted := 0

	tfiles := t.Files()
	if len(tfiles) > 0 && torrent.Files == nil {
		torrent.Files = make([]*File, len(tfiles))
	}
	//merge in files
	for i, f := range tfiles {
		path := f.Path()
		file := torrent.Files[i]
		if file == nil {
			file = &File{Path: path}
			torrent.Files[i] = file
		}
		chunks := f.State()

		file.Size = f.Length()
		file.Chunks = len(chunks)
		completed := 0
		for _, p := range chunks {
			if p.Complete {
				completed++
			}
		}
		file.Completed = completed
		file.Percent = percent(int64(file.Completed), int64(file.Chunks))
		file.Done = (file.Completed == file.Chunks)
		file.f = *f

		totalChunks += file.Chunks
		totalCompleted += file.Completed
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
	torrent.Done = (bytes == torrent.Size)

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

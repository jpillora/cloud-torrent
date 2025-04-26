package engine

import (
	"time"

	atorrent "github.com/anacrolix/torrent"
)

type Torrent struct {
	//anacrolix/torrent
	InfoHash   string
	Name       string
	Loaded     bool
	Downloaded int64
	Size       int64
	Files      []*File
	//cloud torrent
	Started         bool
	Dropped         bool
	Percent         float32
	DownloadRate    float32
	t               *atorrent.Torrent
	updatedAt       time.Time
	FilesToDownload []int
}

type File struct {
	//anacrolix/torrent
	Path      string
	Size      int64
	Chunks    int
	Completed int
	//cloud torrent
	Started      bool
	Percent      float32
	FilePosition int
	f            *atorrent.File
}

func (torrent *Torrent) Update(t *atorrent.Torrent) {
	torrent.Name = t.Name()
	torrent.Loaded = t.Info() != nil
	if torrent.Loaded {
		torrent.updateLoaded(t)
	}
	torrent.t = t
}

// Updates our torrent struct with the latest information from the anacrolix/torrent struct
func (torrent *Torrent) updateLoaded(t *atorrent.Torrent) {
	// Currently this is the total size of torrent in bytes
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
			file = &File{Path: path, FilePosition: i}
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
		file.f = f

		totalChunks += file.Chunks
		totalCompleted += file.Completed
	}

	// Calculate the size of the files we are downloading
	torrentSize := int64(0)
	for _, filePosition := range torrent.FilesToDownload {
		torrentSize += int64(torrent.Files[filePosition].Size)
	}
	torrent.Size = torrentSize

	//cacluate rate
	now := time.Now()
	bytes := t.BytesCompleted()
	torrent.Percent = percent(bytes, torrentSize)
	if !torrent.updatedAt.IsZero() {
		dt := float32(now.Sub(torrent.updatedAt))
		db := float32(bytes - torrent.Downloaded)
		rate := db * (float32(time.Second) / dt)
		if rate >= 0 {
			torrent.DownloadRate = rate
		}
	}
	torrent.Downloaded = bytes
	torrent.updatedAt = now
}

func percent(n, total int64) float32 {
	if total == 0 {
		return float32(0)
	}
	return float32(int(float64(10000)*(float64(n)/float64(total)))) / 100
}

package engine

import (
	"time"

	"github.com/anacrolix/torrent"
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
	Started      bool
	Percent      float32
	DownloadRate float32
	t            *torrent.Torrent
	updatedAt    time.Time
}

type File struct {
	//anacrolix/torrent
	Path      string
	Size      int64
	Chunks    int
	Completed int
	//cloud torrent
	Started bool
	Percent float32
	f       *torrent.File
}

func (torrent *Torrent) Update(t *torrent.Torrent) {
	torrent.Name = t.Name()
	torrent.Loaded = t.Info() != nil
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
		file.Percent = percent(file.Completed, file.Chunks)
		file.f = &f

		totalChunks += file.Chunks
		totalCompleted += file.Completed
	}

	torrent.Percent = percent(totalChunks, totalCompleted)
	//cacluate rate
	now := time.Now()
	bytes := t.BytesCompleted()
	if !torrent.updatedAt.IsZero() {
		dt := float32(now.Sub(torrent.updatedAt))
		db := float32(bytes - torrent.Downloaded)
		torrent.DownloadRate = db * (float32(time.Second) / dt)
	}
	torrent.Downloaded = bytes
	torrent.updatedAt = now

}

func percent(n, total int) float32 {
	if total == 0 {
		return float32(0)
	}
	return float32(int(float32(10000)*(float32(n)/float32(total)))) / 100
}

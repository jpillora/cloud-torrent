package engine

import (
	"log"
	"sync"
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
	IsSeeding     bool
	ManualStarted bool
	Percent       float32
	DownloadRate  float32
	UploadRate    float32
	SeedRatio     float32
	AddedAt       time.Time
	StartedAt     time.Time
	t             *torrent.Torrent
	dropWait      chan struct{}
	updatedAt     time.Time
	cldServer     Server
	sync.Mutex
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
func (torrent *Torrent) updateBase(t *torrent.Torrent) {
	torrent.Lock()
	defer torrent.Unlock()

	if t.Info() != nil {
		torrent.t = t
		torrent.Loaded = true
		torrent.updateStatus()
		torrent.updateConnStat()
	}

	if torrent.Magnet == "" {
		meta := t.Metainfo()
		if ifo, err := meta.UnmarshalInfo(); err == nil {
			magnet := meta.Magnet(nil, &ifo).String()
			torrent.Magnet = magnet
		} else {
			torrent.Magnet = "ERROR{}"
		}
		torrent.Name = t.Name()
	}
}

func (torrent *Torrent) updateConnStat() {
	torrent.Stats = torrent.t.Stats()

	// calculate ratio
	bRead := torrent.Stats.BytesReadData.Int64()
	bWrite := torrent.Stats.BytesWrittenData.Int64()
	if bRead > 0 {
		torrent.SeedRatio = float32(bWrite) / float32(bRead)
	}

	now := time.Now()
	bytes := torrent.t.BytesCompleted()
	ulbytes := torrent.Stats.BytesWrittenData.Int64()

	if !torrent.updatedAt.IsZero() {
		// calculate rate
		dtinv := float32(time.Second) / float32(now.Sub(torrent.updatedAt))

		dldb := float32(bytes - torrent.Downloaded)
		torrent.DownloadRate = dldb * dtinv

		uldb := float32(ulbytes - torrent.Uploaded)
		torrent.UploadRate = uldb * dtinv
	}

	torrent.Downloaded = bytes
	torrent.Uploaded = ulbytes
	torrent.updatedAt = now
}

func (torrent *Torrent) updateStatus() {
	{
		tfiles := torrent.t.Files()
		if len(tfiles) > 0 && torrent.Files == nil {
			torrent.Files = make([]*File, len(tfiles))
		}
		//merge in files
		for i, f := range tfiles {
			path := f.Path()
			file := torrent.Files[i]
			if file == nil {
				file = &File{Path: path, Started: torrent.Started, f: f}
				torrent.Files[i] = file
			}

			file.Size = f.Length()
			file.Completed = f.BytesCompleted()
			file.Percent = percent(file.Completed, file.Size)
			file.Done = (file.Completed == file.Size)
			if file.Done && !file.DoneCmdCalled && !torrent.updatedAt.IsZero() {
				file.DoneCmdCalled = true
				go torrent.callDoneCmd(file.Path, "file", file.Size)
			}
		}
	}

	torrent.Size = torrent.t.Length()
	torrent.Percent = percent(torrent.t.BytesCompleted(), torrent.Size)
	torrent.Done = (torrent.t.BytesMissing() == 0)
	torrent.IsSeeding = torrent.t.Seeding() && torrent.Done

	// this process called at least on second Update calls
	if torrent.Done && !torrent.DoneCmdCalled && !torrent.updatedAt.IsZero() {
		torrent.DoneCmdCalled = true
		go torrent.callDoneCmd(torrent.Name, "torrent", torrent.Size)
	}
}

func percent(n, total int64) float32 {
	if total == 0 {
		return float32(0)
	}
	return float32(int(float64(10000)*(float64(n)/float64(total)))) / 100
}

func (t *Torrent) callDoneCmd(name, tasktype string, size int64) {
	if cmd, err := t.cldServer.DoneCmd(name, t.InfoHash, tasktype,
		size, t.StartedAt.Unix()); err == nil {
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Println("[DoneCmd] Err:", err)
			return
		}
		log.Println("[DoneCmd] Exit:", cmd.ProcessState.ExitCode(), "Output:", string(out))
	}
}

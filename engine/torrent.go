package engine

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

type Torrent struct {
	sync.Mutex

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
	Stats          *torrent.TorrentStats
	Started        bool
	Done           bool
	DoneCmdCalled  bool
	IsQueueing     bool
	IsSeeding      bool
	ManualStarted  bool
	IsAllFilesDone bool
	Percent        float32
	DownloadRate   float32
	UploadRate     float32
	SeedRatio      float32
	AddedAt        time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
	StoppedAt      time.Time
	updatedAt      time.Time
	t              *torrent.Torrent
	e              *Engine
	dropWait       chan struct{}
	cld            Server
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
func (torrent *Torrent) updateOnGotInfo(t *torrent.Torrent) {

	if t.Info() != nil && !torrent.Loaded {
		torrent.t = t
		torrent.Name = t.Name()
		torrent.Loaded = true
		torrent.updateFileStatus()
		torrent.updateTorrentStatus()
		torrent.updateConnStat()

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
}

func (torrent *Torrent) updateConnStat() {
	now := time.Now()
	lastStat := torrent.Stats
	curStat := torrent.t.Stats()

	if lastStat == nil {
		torrent.updatedAt = now
		torrent.Stats = &curStat
		return
	}

	bRead := curStat.BytesReadUsefulData.Int64()
	bWrite := curStat.BytesWrittenData.Int64()

	lRead := lastStat.BytesReadUsefulData.Int64()
	lWrite := lastStat.BytesWrittenData.Int64()

	// download will stop if torrent is done (bRead equals)
	if bRead >= lRead || bWrite > lWrite {

		// calculate ratio
		if bRead > 0 {
			torrent.SeedRatio = float32(bWrite) / float32(bRead)
		} else if torrent.Done {
			torrent.SeedRatio = float32(bWrite) / float32(torrent.Size)
		}

		if lastStat != nil {
			// calculate rate
			dtinv := float32(time.Second) / float32(now.Sub(torrent.updatedAt))

			dldb := float32(bRead - lRead)
			torrent.DownloadRate = dldb * dtinv

			uldb := float32(bWrite - lWrite)
			torrent.UploadRate = uldb * dtinv
		}

		torrent.Downloaded = torrent.t.BytesCompleted()
		torrent.Uploaded = bWrite
		torrent.updatedAt = now
		torrent.Stats = &curStat
	}
}

func (torrent *Torrent) updateFileStatus() {
	if torrent.IsAllFilesDone {
		return
	}

	tfiles := torrent.t.Files()
	if len(tfiles) > 0 && torrent.Files == nil {
		torrent.Files = make([]*File, len(tfiles))
	}

	//merge in files
	doneFlag := true
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
		if file.Done && !file.DoneCmdCalled {
			file.DoneCmdCalled = true
			go torrent.callDoneCmd(file.Path, "file", file.Size)
		}
		if !file.Done {
			doneFlag = false
		}
	}

	torrent.IsAllFilesDone = doneFlag
}

func (torrent *Torrent) updateTorrentStatus() {
	torrent.Size = torrent.t.Length()
	torrent.Percent = percent(torrent.t.BytesCompleted(), torrent.Size)
	torrent.Done = (torrent.t.BytesMissing() == 0)
	torrent.IsSeeding = torrent.t.Seeding() && torrent.Done

	// this process called at least on second Update calls
	if torrent.Done && !torrent.DoneCmdCalled {
		torrent.DoneCmdCalled = true
		torrent.FinishedAt = time.Now()
		log.Println("[TaskFinished]", torrent.InfoHash)
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

	if cmd, env, err := t.e.config.GetCmdConfig(); err == nil {
		cmd := exec.Command(cmd)
		ih := t.InfoHash
		cmd.Env = append(env,
			fmt.Sprintf("CLD_RESTAPI=%s", t.cld.GetStrAttribute("RestAPI")),
			fmt.Sprintf("CLD_PATH=%s", name),
			fmt.Sprintf("CLD_HASH=%s", ih),
			fmt.Sprintf("CLD_TYPE=%s", tasktype),
			fmt.Sprintf("CLD_SIZE=%d", size),
			fmt.Sprintf("CLD_STARTTS=%d", t.StartedAt.Unix()),
			fmt.Sprintf("CLD_FILENUM=%d", len(t.Files)),
		)
		sout, _ := cmd.StdoutPipe()
		serr, _ := cmd.StderrPipe()
		log.Printf("[DoneCmd:%s]%sCMD:`%s' ENV:%s", tasktype, ih, cmd.String(), cmd.Env)
		if err := cmd.Start(); err != nil {
			log.Printf("[DoneCmd:%s]%sERR: %v", tasktype, ih, err)
			return
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go cmdScanLine(sout, &wg, fmt.Sprintf("[DoneCmd:%s]%sO:", log.filteredArg(tasktype, ih)...))
		go cmdScanLine(serr, &wg, fmt.Sprintf("[DoneCmd:%s]%sE:", log.filteredArg(tasktype, ih)...))
		wg.Wait()

		// call Wait will close pipes above
		if err := cmd.Wait(); err != nil {
			log.Printf("[DoneCmd:%s]%sERR: %v", tasktype, ih, err)
			return
		}

		log.Printf("[DoneCmd:%s]%sExit code: %d", tasktype, ih, cmd.ProcessState.ExitCode())
	} else {
		log.Println("[DoneCmd]", t.InfoHash, err)
	}
}

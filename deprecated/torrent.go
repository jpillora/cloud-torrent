package engine

import (
	"errors"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/jpillora/cloud-torrent/storage"
)

func NewTorrent(ih string, storage *storage.Storage, sortConfig *MediaSortConfig) *Torrent {
	return &Torrent{
		InfoHash:   ih,
		storage:    storage,
		sortConfig: sortConfig,
	}
}

//Torrent is converted to JSON and sent to the frontend
type Torrent struct {
	//anacrolix/torrent
	at         *torrent.Torrent
	InfoHash   string
	Name       string
	Loaded     bool
	Downloaded int64
	Size       int64
	Files      []*File
	//state
	files      map[string]*File
	loadedMut  sync.Mutex
	updatedAt  time.Time
	filesMut   sync.Mutex
	storage    *storage.Storage
	sortConfig *MediaSortConfig
	//cloud torrent
	Started      bool
	Dropped      bool
	Percent      float32
	DownloadRate float32
}

func (torrent *Torrent) init(info *metainfo.Info) {
	torrent.loadedMut.Lock()
	defer torrent.loadedMut.Unlock()
	if torrent.Loaded {
		return
	}
	// torrent.InfoHash = info. .HexString()
	// torrent.info = info
	torrent.Name = info.Name
	torrent.Size = info.TotalLength()
	numFiles := len(info.Files)
	torrent.files = map[string]*File{}
	torrent.Files = make([]*File, numFiles)
	wg := sync.WaitGroup{}
	wg.Add(numFiles)
	for i, f := range info.Files {
		go func(i int, f metainfo.FileInfo) {
			file := NewFile(strings.Join(f.Path, "/"), f.Length)
			file.sort(torrent.sortConfig)
			torrent.Files[i] = file
			torrent.files[file.Path] = file
			wg.Done()
		}(i, f)
	}
	wg.Wait()
	torrent.Loaded = true
}

func (torrent *Torrent) Get(path string) (*File, bool) {
	f, ok := torrent.files[path]
	return f, ok
}

func (torrent *Torrent) Update(tt torrent.Torrent) {
	if info := tt.Info(); info != nil {
		torrent.init(&info.Info)
	}
	torrent.filesMut.Lock()
	if torrent.Loaded {
		//update file progress
		for i, f := range tt.Files() {
			file := torrent.Files[i]
			chunks := f.State()
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
		}
	}
	//cacluate rate
	now := time.Now()
	bytesDownloaded := tt.BytesCompleted()
	torrent.Percent = percent(bytesDownloaded, torrent.Size)
	if !torrent.updatedAt.IsZero() {
		dt := float32(now.Sub(torrent.updatedAt))
		db := float32(bytesDownloaded - torrent.Downloaded)
		rate := db * (float32(time.Second) / dt)
		if rate >= 0 {
			torrent.DownloadRate = rate
		}
	}
	torrent.Downloaded = bytesDownloaded
	torrent.updatedAt = now
	// torrent.tt = tt
	torrent.filesMut.Unlock()
}

func percent(n, total int64) float32 {
	if total == 0 {
		return float32(0)
	}
	return float32(int(float64(10000)*(float64(n)/float64(total)))) / 100
}

func (torrent *Torrent) Start() error {
	return nil
}

func (torrent *Torrent) Stop() error {
	return nil
}

func (torrent *Torrent) File(off int64) (*File, int64) {
	n := int64(0)
	for _, f := range torrent.Files {
		if off > n && n+f.Size < off {
			foff := off - n
			return f, foff
		}
	}
	return nil, 0
}

func (torrent *Torrent) ReadAt(p []byte, off int64) (n int, err error) {
	f, foff := torrent.File(off)
	if f == nil {
		return 0, errors.New("missing file")
	}
	log.Printf("[%s] read at (%d)", torrent.Name, len(p))
	time.Sleep(5 * time.Second)
	return f.ReadAt(p, foff)
}

func (torrent *Torrent) WriteAt(p []byte, off int64) (n int, err error) {
	f, foff := torrent.File(off)
	if f == nil {
		return 0, errors.New("missing file")
	}
	log.Printf("[%s] write at", torrent.Name)
	return f.WriteAt(p, foff)
}

func (torrent *Torrent) Close() {
	log.Printf("[%s] close requested", torrent.Name)
	return
}

// If the data isn'tt available, err should be io.ErrUnexpectedEOF.
func (torrent *Torrent) WriteSectionTo(w io.Writer, off, n int64) (written int64, err error) {
	f, foff := torrent.File(off)
	if f == nil {
		return 0, errors.New("missing file")
	}
	log.Printf("[%s] write to section", torrent.Name)
	return f.WriteSectionTo(w, foff, n)
}

// We believe the piece data will pass a hash check.
func (torrent *Torrent) PieceCompleted(index int) error {
	// l := torrent.info.NumPieces()
	//
	// torrent.info.Pieces[index]
	// p :=
	// f, off := torrent.File(p.Offset())
	// if f == nil {
	// 	return errors.New("missing file")
	// }
	// log.Printf("[%s] set complete '%s' %d", torrent.Name, f.Path, off)
	return nil
}

// Returns true if the piece is complete.
func (torrent *Torrent) PieceComplete(index int) bool {
	// if torrent.info == nil {
	// 	return false
	// }
	// p := torrent.info.Piece(index)
	// f, _ := torrent.File(p.Offset())
	// if f == nil {
	// 	return false
	// }
	// log.Printf("[%s] is complete %d?", torrent.Name, index)
	return false
}

package metainfo

import (
	"crypto/sha1"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/bencode"
)

//----------------------------------------------------------------------------
// Build
//----------------------------------------------------------------------------

// The Builder type is responsible for .torrent files construction. Just
// instantiate it, call necessary methods and then call the .Build method. While
// waiting for completion you can use 'status' channel to get status reports.
type Builder struct {
	batch_state
	filesmap map[string]bool
}

// Adds a file to the builder queue. You may add one or more files.
func (b *Builder) AddFile(filename string) {
	if b.filesmap == nil {
		b.filesmap = make(map[string]bool)
	}

	filename, err := filepath.Abs(filename)
	if err != nil {
		panic(err)
	}
	b.filesmap[filename] = true
}

// Defines a name of the future torrent file. For single file torrents it's the
// recommended name of the contained file. For multiple files torrents it's the
// recommended name of the directory in which all of them will be
// stored. Calling this function is not required. In case if no name was
// specified, the builder will try to automatically assign it. It will use the
// name of the file if there is only one file in the queue or it will try to
// find the rightmost common directory of all the queued files and use its name as
// a torrent name. In case if name cannot be assigned automatically, it will use
// "unknown" as a torrent name.
func (b *Builder) SetName(name string) {
	b.name = name
}

// Sets the length of a piece in the torrent file in bytes. The default is
// 256kb.
func (b *Builder) SetPieceLength(length int64) {
	b.piece_length = length
}

// Sets the "private" flag. The default is false.
func (b *Builder) SetPrivate(v bool) {
	b.private = v
}

// Add announce URL group. TODO: better explanation.
func (b *Builder) AddAnnounceGroup(group []string) {
	b.announce_list = append(b.announce_list, group)
}

// Add DHT nodes URLs for trackerless mode
func (b *Builder) AddDhtNodes(group []string) {
	b.node_list = append(b.node_list, group...)
}

// Sets creation date. The default is time.Now() when the .Build method was
// called.
func (b *Builder) SetCreationDate(date time.Time) {
	b.creation_date = date
}

// Sets the comment. The default is no comment.
func (b *Builder) SetComment(comment string) {
	b.comment = comment
}

// Sets the "created by" parameter. The default is "libtorgo".
func (b *Builder) SetCreatedBy(createdby string) {
	b.created_by = createdby
}

// Sets the "encoding" parameter. The default is "UTF-8".
func (b *Builder) SetEncoding(encoding string) {
	b.encoding = encoding
}

// Add WebSeed URL to the list.
func (b *Builder) AddWebSeedURL(url string) {
	b.urls = append(b.urls, url)
}

// Finalizes the Builder state and makes a Batch out of it. After calling that
// method, Builder becomes empty and you can use it to create another Batch if
// you will.
func (b *Builder) Submit() (*Batch, error) {
	err := b.check_parameters()
	if err != nil {
		return nil, err
	}
	b.set_defaults()

	batch := &Batch{
		batch_state: b.batch_state,
	}

	const non_regular = os.ModeDir | os.ModeSymlink |
		os.ModeDevice | os.ModeNamedPipe | os.ModeSocket

	// convert a map to a slice, calculate sizes and split paths
	batch.total_size = 0
	batch.files = make([]file, 0, 10)
	for f, _ := range b.filesmap {
		var file file
		fi, err := os.Stat(f)
		if err != nil {
			return nil, err
		}

		if fi.Mode()&non_regular != 0 {
			return nil, errors.New(f + " is not a regular file")
		}

		file.abspath = f
		file.splitpath = split_path(f)
		file.size = fi.Size()
		batch.files = append(batch.files, file)
		batch.total_size += file.size
	}

	// find the rightmost common directory
	if len(batch.files) == 1 {
		sp := batch.files[0].splitpath
		batch.default_name = sp[len(sp)-1]
	} else {
		common := batch.files[0].splitpath
		for _, f := range batch.files {
			if len(common) > len(f.splitpath) {
				common = common[:len(f.splitpath)]
			}

			for i, n := 0, len(common); i < n; i++ {
				if common[i] != f.splitpath[i] {
					common = common[:i]
					break
				}
			}

			if len(common) == 0 {
				break
			}
		}

		if len(common) == 0 {
			return nil, errors.New("no common rightmost folder was found for a set of queued files")
		}

		// found the common folder, let's strip that part from splitpath
		// and setup the default name
		batch.default_name = common[len(common)-1]

		lcommon := len(common)
		for i := range batch.files {
			f := &batch.files[i]
			f.splitpath = f.splitpath[lcommon:]
		}

		// and finally sort the files
		sort.Sort(file_slice(batch.files))
	}

	// reset the builder state
	b.batch_state = batch_state{}
	b.filesmap = nil

	return batch, nil
}

func (b *Builder) set_defaults() {
	if b.piece_length == 0 {
		b.piece_length = 256 * 1024
	}

	if b.creation_date.IsZero() {
		b.creation_date = time.Now()
	}

	if b.created_by == "" {
		b.created_by = "libtorgo"
	}

	if b.encoding == "" {
		b.encoding = "UTF-8"
	}
}

func emptyStringsFiltered(ss []string) (ret []string) {
	for _, s := range ss {
		if s != "" {
			ret = append(ret, s)
		}
	}
	return
}

func (b *Builder) check_parameters() error {
	// should be at least one file
	if len(b.filesmap) == 0 {
		return errors.New("no files were queued")
	}

	// let's clean up the announce_list and node_list
	b.announce_list = cleanUpLists(b.announce_list)
	b.node_list = emptyStringsFiltered(b.node_list)

	if len(b.announce_list) == 0 && len(b.node_list) == 0 {
		return errors.New("no announce group or DHT nodes specified")
	}

	// Either the node_list or announce_list can be present
	// Never the both!
	if len(b.announce_list) > 0 && len(b.node_list) > 0 {
		return errors.New("announce group and nodes are mutually exclusive")
	}

	// and clean up the urls
	b.urls = remove_empty_strings(b.urls)

	return nil
}

func cleanUpLists(list [][]string) [][]string {
	newList := make([][]string, 0, len(list))
	for _, l := range list {
		l = remove_empty_strings(l)

		// discard empty announce groups
		if len(l) == 0 {
			continue
		}
		newList = append(newList, l)
	}
	return newList
}

//----------------------------------------------------------------------------
// Batch
//----------------------------------------------------------------------------

// Batch represents a snapshot of a builder state, ready for transforming it
// into a torrent file. Note that Batch contains two accessor methods you might
// be interested in. The TotalSize is the total size of all the files queued for
// hashing, you will use it for status reporting. The DefaultName is an
// automatically determined name of the torrent metainfo, you might want to use
// it for naming the .torrent file itself.
type Batch struct {
	batch_state
	files        []file
	total_size   int64
	default_name string
}

// Get a total size of all the files queued for hashing. Useful in conjunction
// with status reports.
func (b *Batch) TotalSize() int64 {
	return b.total_size
}

// Get an automatically determined name of the future torrent metainfo. You can
// use it for a .torrent file in case user hasn't provided it specifically.
func (b *Batch) DefaultName() string {
	return b.default_name
}

// Starts a process of building the torrent file. This function does everything
// in a separate goroutine and uses up to 'nworkers' of goroutines to perform
// SHA1 hashing. Therefore it will return almost immedately. It returns two
// channels, the first one is for completion awaiting, the second one is for
// getting status reports. Status report is a number of bytes hashed, you can
// get the total amount of bytes by inspecting the Batch.TotalSize method return
// value.
func (b *Batch) Start(w io.Writer, nworkers int) (<-chan error, <-chan int64) {
	if nworkers <= 0 {
		nworkers = 1
	}

	completion := make(chan error)
	status := make(chan int64)

	go func() {
		// prepare workers
		workers := make([]*worker, nworkers)
		free_workers := make(chan *worker, nworkers)
		for i := 0; i < nworkers; i++ {
			workers[i] = new_worker(free_workers)
		}
		stop_workers := func() {
			for _, w := range workers {
				w.stop()
			}
			for _, w := range workers {
				w.wait_for_stop()
			}
		}

		// prepare files for reading
		fr := files_reader{files: b.files}
		npieces := (b.total_size + b.piece_length - 1) / b.piece_length
		b.pieces = make([]byte, 20*npieces)
		hashed := int64(0)

		// read all the pieces passing them to workers for hashing
		var data []byte
		for i := int64(0); i < npieces; i++ {
			if data == nil {
				data = make([]byte, b.piece_length)
			}

			nr, err := fr.Read(data)
			if err != nil {
				// EOF is not an eror if it was the last piece
				if err == io.EOF {
					if i != npieces-1 {
						stop_workers()
						completion <- err
						return
					}
				} else {
					stop_workers()
					completion <- err
					return
				}
			}

			// cut the data slice to the amount of actual data read
			data = data[:nr]
			w := <-free_workers
			data = w.queue(data, b.pieces[20*i:20*i+20])

			// update and try to send the status report
			if data != nil {
				hashed += int64(len(data))
				data = data[:cap(data)]

				select {
				case status <- hashed:
				default:
				}
			}
		}
		stop_workers()

		// at this point the hash was calculated and we're ready to
		// write the torrent file
		err := b.write_torrent(w)
		if err != nil {
			completion <- err
			return
		}
		completion <- nil
	}()
	return completion, status
}

func (b *Batch) write_torrent(w io.Writer) error {
	var td MetaInfo

	// Either announce or node lists are allowed - not both
	if len(b.announce_list) != 0 {
		td.Announce = b.announce_list[0][0]
		if len(b.announce_list) != 1 || len(b.announce_list[0]) != 1 {
			td.AnnounceList = b.announce_list
		}
	}

	missinggo.CastSlice(&td.Nodes, b.node_list)
	td.CreationDate = b.creation_date.Unix()
	td.Comment = b.comment
	td.CreatedBy = b.created_by
	td.Encoding = b.encoding
	switch {
	case len(b.urls) == 0:
	case len(b.urls) == 1:
		td.URLList = b.urls[0]
	default:
		td.URLList = b.urls
	}

	td.Info.PieceLength = b.piece_length
	td.Info.Pieces = b.pieces
	if b.name == "" {
		td.Info.Name = b.default_name
	} else {
		td.Info.Name = b.name
	}
	if len(b.files) == 1 {
		td.Info.Length = b.files[0].size
	} else {
		td.Info.Files = make([]FileInfo, len(b.files))
		for i, f := range b.files {
			td.Info.Files[i] = FileInfo{
				Path:   f.splitpath,
				Length: f.size,
			}
		}
	}
	td.Info.Private = b.private

	e := bencode.NewEncoder(w)
	return e.Encode(&td)
}

//----------------------------------------------------------------------------
// misc stuff
//----------------------------------------------------------------------------

// splits path into components (dirs and files), works only on absolute paths
func split_path(path string) []string {
	var dir, file string
	s := make([]string, 0, 5)

	dir = path
	for {
		dir, file = filepath.Split(filepath.Clean(dir))
		if file == "" {
			break
		}
		s = append(s, file)
	}

	// reverse the slice
	for i, n := 0, len(s)/2; i < n; i++ {
		i2 := len(s) - i - 1
		s[i], s[i2] = s[i2], s[i]
	}

	return s
}

// just a common data between the Builder and the Batch
type batch_state struct {
	name          string
	piece_length  int64
	pieces        []byte
	private       bool
	announce_list [][]string
	node_list     []string
	creation_date time.Time
	comment       string
	created_by    string
	encoding      string
	urls          []string
}

type file struct {
	abspath   string
	splitpath []string
	size      int64
}

type file_slice []file

func (s file_slice) Len() int           { return len(s) }
func (s file_slice) Less(i, j int) bool { return s[i].abspath < s[j].abspath }
func (s file_slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func remove_empty_strings(slice []string) []string {
	j := 0
	for i, n := 0, len(slice); i < n; i++ {
		if slice[i] == "" {
			continue
		}
		slice[j] = slice[i]
		j++
	}
	return slice[:j]
}

//----------------------------------------------------------------------------
// worker
//----------------------------------------------------------------------------

type worker struct {
	msgbox chan bool
	hash   hash.Hash

	// request
	sha1 []byte
	data []byte
}

// returns existing 'data'
func (w *worker) queue(data, sha1 []byte) []byte {
	d := w.data
	w.data = data
	w.sha1 = sha1
	w.msgbox <- false
	return d
}

func (w *worker) stop() {
	w.msgbox <- true
}

func (w *worker) wait_for_stop() {
	<-w.msgbox
}

func new_worker(out chan<- *worker) *worker {
	w := &worker{
		msgbox: make(chan bool),
		hash:   sha1.New(),
	}
	go func() {
		var sha1 [20]byte
		for {
			if <-w.msgbox {
				w.msgbox <- true
				return
			}
			w.hash.Reset()
			w.hash.Write(w.data)
			w.hash.Sum(sha1[:0])
			copy(w.sha1, sha1[:])
			out <- w
		}
	}()
	out <- w
	return w
}

//----------------------------------------------------------------------------
// files_reader
//----------------------------------------------------------------------------

type files_reader struct {
	files   []file
	cur     int
	curfile *os.File
	off     int64
}

func (f *files_reader) Read(data []byte) (int, error) {
	if f.cur >= len(f.files) {
		return 0, io.EOF
	}

	if len(data) == 0 {
		return 0, nil
	}

	read := 0
	for len(data) > 0 {
		file := &f.files[f.cur]
		if f.curfile == nil {
			var err error
			f.curfile, err = os.Open(file.abspath)
			if err != nil {
				return read, err
			}
		}

		// we need to read up to 'len(data)' bytes from current file
		n := int64(len(data))

		// unless there is not enough data in this file
		if file.size-f.off < n {
			n = file.size - f.off
		}

		// if there is no data in this file, try next one
		if n == 0 {
			err := f.curfile.Close()
			if err != nil {
				return read, err
			}

			f.curfile = nil
			f.off = 0
			f.cur++
			if f.cur >= len(f.files) {
				return read, io.EOF
			}
			continue
		}

		// read, handle errors
		nr, err := f.curfile.Read(data[:n])
		read += nr
		f.off += int64(nr)
		if err != nil {
			return read, err
		}

		// ok, we've read nr bytes out of len(data), cut the data slice
		data = data[nr:]
	}

	return read, nil
}

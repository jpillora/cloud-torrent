package torrent

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/dht"
	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/filecache"
	"github.com/anacrolix/missinggo/pubsub"
	"github.com/anacrolix/utp"
	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/internal/testutil"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Llongfile)
}

var TestingConfig = Config{
	ListenAddr:      "localhost:0",
	NoDHT:           true,
	DisableTrackers: true,
	DataDir:         "/tmp/anacrolix",
	DHTConfig: dht.ServerConfig{
		NoDefaultBootstrap: true,
	},
	Debug: true,
}

func TestClientDefault(t *testing.T) {
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	cl.Close()
}

func TestAddDropTorrent(t *testing.T) {
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	defer cl.Close()
	dir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(dir)
	tt, new, err := cl.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	require.NoError(t, err)
	assert.True(t, new)
	tt.SetMaxEstablishedConns(0)
	tt.SetMaxEstablishedConns(1)
	tt.Drop()
}

func TestAddTorrentNoSupportedTrackerSchemes(t *testing.T) {
	t.SkipNow()
}

func TestAddTorrentNoUsableURLs(t *testing.T) {
	t.SkipNow()
}

func TestAddPeersToUnknownTorrent(t *testing.T) {
	t.SkipNow()
}

func TestPieceHashSize(t *testing.T) {
	if pieceHash.Size() != 20 {
		t.FailNow()
	}
}

func TestTorrentInitialState(t *testing.T) {
	dir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(dir)
	tor := &Torrent{
		infoHash:          mi.HashInfoBytes(),
		pieceStateChanges: pubsub.NewPubSub(),
	}
	tor.chunkSize = 2
	tor.storageOpener = storage.NewClient(storage.NewFile("/dev/null"))
	// Needed to lock for asynchronous piece verification.
	tor.cl = new(Client)
	err := tor.setInfoBytes(mi.InfoBytes)
	require.NoError(t, err)
	require.Len(t, tor.pieces, 3)
	tor.pendAllChunkSpecs(0)
	tor.cl.mu.Lock()
	assert.EqualValues(t, 3, tor.pieceNumPendingChunks(0))
	tor.cl.mu.Unlock()
	assert.EqualValues(t, chunkSpec{4, 1}, chunkIndexSpec(2, tor.pieceLength(0), tor.chunkSize))
}

func TestUnmarshalPEXMsg(t *testing.T) {
	var m peerExchangeMessage
	if err := bencode.Unmarshal([]byte("d5:added12:\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0ce"), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Added) != 2 {
		t.FailNow()
	}
	if m.Added[0].Port != 0x506 {
		t.FailNow()
	}
}

func TestReducedDialTimeout(t *testing.T) {
	for _, _case := range []struct {
		Max             time.Duration
		HalfOpenLimit   int
		PendingPeers    int
		ExpectedReduced time.Duration
	}{
		{nominalDialTimeout, 40, 0, nominalDialTimeout},
		{nominalDialTimeout, 40, 1, nominalDialTimeout},
		{nominalDialTimeout, 40, 39, nominalDialTimeout},
		{nominalDialTimeout, 40, 40, nominalDialTimeout / 2},
		{nominalDialTimeout, 40, 80, nominalDialTimeout / 3},
		{nominalDialTimeout, 40, 4000, nominalDialTimeout / 101},
	} {
		reduced := reducedDialTimeout(_case.Max, _case.HalfOpenLimit, _case.PendingPeers)
		expected := _case.ExpectedReduced
		if expected < minDialTimeout {
			expected = minDialTimeout
		}
		if reduced != expected {
			t.Fatalf("expected %s, got %s", _case.ExpectedReduced, reduced)
		}
	}
}

func TestUTPRawConn(t *testing.T) {
	l, err := utp.NewSocket("udp", "")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			_, err := l.Accept()
			if err != nil {
				break
			}
		}
	}()
	// Connect a UTP peer to see if the RawConn will still work.
	s, _ := utp.NewSocket("udp", "")
	defer s.Close()
	utpPeer, err := s.Dial(fmt.Sprintf("localhost:%d", missinggo.AddrPort(l.Addr())))
	if err != nil {
		t.Fatalf("error dialing utp listener: %s", err)
	}
	defer utpPeer.Close()
	peer, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()

	msgsReceived := 0
	// How many messages to send. I've set this to double the channel buffer
	// size in the raw packetConn.
	const N = 200
	readerStopped := make(chan struct{})
	// The reader goroutine.
	go func() {
		defer close(readerStopped)
		b := make([]byte, 500)
		for i := 0; i < N; i++ {
			n, _, err := l.ReadFrom(b)
			if err != nil {
				t.Fatalf("error reading from raw conn: %s", err)
			}
			msgsReceived++
			var d int
			fmt.Sscan(string(b[:n]), &d)
			if d != i {
				log.Printf("got wrong number: expected %d, got %d", i, d)
			}
		}
	}()
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("localhost:%d", missinggo.AddrPort(l.Addr())))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < N; i++ {
		_, err := peer.WriteTo([]byte(fmt.Sprintf("%d", i)), udpAddr)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Microsecond)
	}
	select {
	case <-readerStopped:
	case <-time.After(time.Second):
		t.Fatal("reader timed out")
	}
	if msgsReceived != N {
		t.Fatalf("messages received: %d", msgsReceived)
	}
}

func TestTwoClientsArbitraryPorts(t *testing.T) {
	for i := 0; i < 2; i++ {
		cl, err := NewClient(&TestingConfig)
		if err != nil {
			t.Fatal(err)
		}
		defer cl.Close()
	}
}

func TestAddDropManyTorrents(t *testing.T) {
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	defer cl.Close()
	for i := range iter.N(1000) {
		var spec TorrentSpec
		binary.PutVarint(spec.InfoHash[:], int64(i))
		tt, new, err := cl.AddTorrentSpec(&spec)
		assert.NoError(t, err)
		assert.True(t, new)
		defer tt.Drop()
	}
}

type FileCacheClientStorageFactoryParams struct {
	Capacity    int64
	SetCapacity bool
	Wrapper     func(*filecache.Cache) storage.ClientImpl
}

func NewFileCacheClientStorageFactory(ps FileCacheClientStorageFactoryParams) storageFactory {
	return func(dataDir string) storage.ClientImpl {
		fc, err := filecache.NewCache(dataDir)
		if err != nil {
			panic(err)
		}
		if ps.SetCapacity {
			fc.SetCapacity(ps.Capacity)
		}
		return ps.Wrapper(fc)
	}
}

type storageFactory func(string) storage.ClientImpl

func TestClientTransferDefault(t *testing.T) {
	testClientTransfer(t, testClientTransferParams{
		ExportClientStatus: true,
		LeecherStorage: NewFileCacheClientStorageFactory(FileCacheClientStorageFactoryParams{
			Wrapper: fileCachePieceResourceStorage,
		}),
	})
}

func TestClientTransferRateLimitedUpload(t *testing.T) {
	started := time.Now()
	testClientTransfer(t, testClientTransferParams{
		// We are uploading 13 bytes (the length of the greeting torrent). The
		// chunks are 2 bytes in length. Then the smallest burst we can run
		// with is 2. Time taken is (13-burst)/rate.
		SeederUploadRateLimiter: rate.NewLimiter(11, 2),
	})
	require.True(t, time.Since(started) > time.Second)
}

func TestClientTransferRateLimitedDownload(t *testing.T) {
	testClientTransfer(t, testClientTransferParams{
		LeecherDownloadRateLimiter: rate.NewLimiter(512, 512),
	})
}

func fileCachePieceResourceStorage(fc *filecache.Cache) storage.ClientImpl {
	return storage.NewResourcePieces(fc.AsResourceProvider())
}

func TestClientTransferSmallCache(t *testing.T) {
	testClientTransfer(t, testClientTransferParams{
		LeecherStorage: NewFileCacheClientStorageFactory(FileCacheClientStorageFactoryParams{
			SetCapacity: true,
			// Going below the piece length means it can't complete a piece so
			// that it can be hashed.
			Capacity: 5,
			Wrapper:  fileCachePieceResourceStorage,
		}),
		SetReadahead: true,
		// Can't readahead too far or the cache will thrash and drop data we
		// thought we had.
		Readahead:          0,
		ExportClientStatus: true,
	})
}

func TestClientTransferVarious(t *testing.T) {
	// Leecher storage
	for _, ls := range []storageFactory{
		NewFileCacheClientStorageFactory(FileCacheClientStorageFactoryParams{
			Wrapper: fileCachePieceResourceStorage,
		}),
		storage.NewBoltDB,
	} {
		// Seeder storage
		for _, ss := range []func(string) storage.ClientImpl{
			storage.NewFile,
			storage.NewMMap,
		} {
			for _, responsive := range []bool{false, true} {
				testClientTransfer(t, testClientTransferParams{
					Responsive:     responsive,
					SeederStorage:  ss,
					LeecherStorage: ls,
				})
				for _, readahead := range []int64{-1, 0, 1, 2, 3, 4, 5, 6, 9, 10, 11, 12, 13, 14, 15, 20} {
					testClientTransfer(t, testClientTransferParams{
						SeederStorage:  ss,
						Responsive:     responsive,
						SetReadahead:   true,
						Readahead:      readahead,
						LeecherStorage: ls,
					})
				}
			}
		}
	}
}

type testClientTransferParams struct {
	Responsive                 bool
	Readahead                  int64
	SetReadahead               bool
	ExportClientStatus         bool
	LeecherStorage             func(string) storage.ClientImpl
	SeederStorage              func(string) storage.ClientImpl
	SeederUploadRateLimiter    *rate.Limiter
	LeecherDownloadRateLimiter *rate.Limiter
}

// Creates a seeder and a leecher, and ensures the data transfers when a read
// is attempted on the leecher.
func testClientTransfer(t *testing.T, ps testClientTransferParams) {
	greetingTempDir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(greetingTempDir)
	// Create seeder and a Torrent.
	cfg := TestingConfig
	cfg.Seed = true
	cfg.UploadRateLimiter = ps.SeederUploadRateLimiter
	// cfg.ListenAddr = "localhost:4000"
	if ps.SeederStorage != nil {
		cfg.DefaultStorage = ps.SeederStorage(greetingTempDir)
	} else {
		cfg.DataDir = greetingTempDir
	}
	seeder, err := NewClient(&cfg)
	require.NoError(t, err)
	defer seeder.Close()
	if ps.ExportClientStatus {
		testutil.ExportStatusWriter(seeder, "s")
	}
	// seederTorrent, new, err := seeder.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	_, new, err := seeder.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	require.NoError(t, err)
	assert.True(t, new)
	// Create leecher and a Torrent.
	leecherDataDir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(leecherDataDir)
	if ps.LeecherStorage == nil {
		cfg.DataDir = leecherDataDir
	} else {
		cfg.DefaultStorage = ps.LeecherStorage(leecherDataDir)
	}
	cfg.DownloadRateLimiter = ps.LeecherDownloadRateLimiter
	// cfg.ListenAddr = "localhost:4001"
	leecher, err := NewClient(&cfg)
	require.NoError(t, err)
	defer leecher.Close()
	if ps.ExportClientStatus {
		testutil.ExportStatusWriter(leecher, "l")
	}
	leecherGreeting, new, err := leecher.AddTorrentSpec(func() (ret *TorrentSpec) {
		ret = TorrentSpecFromMetaInfo(mi)
		ret.ChunkSize = 2
		ret.Storage = storage.NewFile(leecherDataDir)
		return
	}())
	require.NoError(t, err)
	assert.True(t, new)
	// Now do some things with leecher and seeder.
	addClientPeer(leecherGreeting, seeder)
	r := leecherGreeting.NewReader()
	defer r.Close()
	if ps.Responsive {
		r.SetResponsive()
	}
	if ps.SetReadahead {
		r.SetReadahead(ps.Readahead)
	}
	assertReadAllGreeting(t, r)
	// After one read through, we can assume certain torrent statistics.
	// These are not a strict requirement. It is however interesting to
	// follow.
	// t.Logf("%#v", seederTorrent.Stats())
	// assert.EqualValues(t, 13, seederTorrent.Stats().DataBytesWritten)
	// assert.EqualValues(t, 8, seederTorrent.Stats().ChunksWritten)
	// assert.EqualValues(t, 13, leecherGreeting.Stats().DataBytesRead)
	// assert.EqualValues(t, 8, leecherGreeting.Stats().ChunksRead)
	// Read through again for the cases where the torrent data size exceeds
	// the size of the cache.
	assertReadAllGreeting(t, r)
}

func assertReadAllGreeting(t *testing.T, r io.ReadSeeker) {
	pos, err := r.Seek(0, os.SEEK_SET)
	assert.NoError(t, err)
	assert.EqualValues(t, 0, pos)
	_greeting, err := ioutil.ReadAll(r)
	assert.NoError(t, err)
	assert.EqualValues(t, testutil.GreetingFileContents, _greeting)
}

// Check that after completing leeching, a leecher transitions to a seeding
// correctly. Connected in a chain like so: Seeder <-> Leecher <-> LeecherLeecher.
func TestSeedAfterDownloading(t *testing.T) {
	greetingTempDir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(greetingTempDir)
	cfg := TestingConfig
	cfg.Seed = true
	cfg.DataDir = greetingTempDir
	seeder, err := NewClient(&cfg)
	require.NoError(t, err)
	defer seeder.Close()
	testutil.ExportStatusWriter(seeder, "s")
	seeder.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	cfg.DataDir, err = ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(cfg.DataDir)
	leecher, err := NewClient(&cfg)
	require.NoError(t, err)
	defer leecher.Close()
	testutil.ExportStatusWriter(leecher, "l")
	cfg.Seed = false
	// cfg.TorrentDataOpener = nil
	cfg.DataDir, err = ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(cfg.DataDir)
	leecherLeecher, _ := NewClient(&cfg)
	defer leecherLeecher.Close()
	testutil.ExportStatusWriter(leecherLeecher, "ll")
	leecherGreeting, _, _ := leecher.AddTorrentSpec(func() (ret *TorrentSpec) {
		ret = TorrentSpecFromMetaInfo(mi)
		ret.ChunkSize = 2
		return
	}())
	llg, _, _ := leecherLeecher.AddTorrentSpec(func() (ret *TorrentSpec) {
		ret = TorrentSpecFromMetaInfo(mi)
		ret.ChunkSize = 3
		return
	}())
	// Simultaneously DownloadAll in Leecher, and read the contents
	// consecutively in LeecherLeecher. This non-deterministically triggered a
	// case where the leecher wouldn't unchoke the LeecherLeecher.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r := llg.NewReader()
		defer r.Close()
		b, err := ioutil.ReadAll(r)
		require.NoError(t, err)
		assert.EqualValues(t, testutil.GreetingFileContents, b)
	}()
	addClientPeer(leecherGreeting, seeder)
	addClientPeer(leecherGreeting, leecherLeecher)
	wg.Add(1)
	go func() {
		defer wg.Done()
		leecherGreeting.DownloadAll()
		leecher.WaitAll()
	}()
	wg.Wait()
}

func TestMergingTrackersByAddingSpecs(t *testing.T) {
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	defer cl.Close()
	spec := TorrentSpec{}
	T, new, _ := cl.AddTorrentSpec(&spec)
	if !new {
		t.FailNow()
	}
	spec.Trackers = [][]string{{"http://a"}, {"udp://b"}}
	_, new, _ = cl.AddTorrentSpec(&spec)
	assert.False(t, new)
	assert.EqualValues(t, [][]string{{"http://a"}, {"udp://b"}}, T.metainfo.AnnounceList)
	// Because trackers are disabled in TestingConfig.
	assert.EqualValues(t, 0, len(T.trackerAnnouncers))
}

type badStorage struct{}

func (bs badStorage) OpenTorrent(*metainfo.Info, metainfo.Hash) (storage.TorrentImpl, error) {
	return bs, nil
}

func (bs badStorage) Close() error {
	return nil
}

func (bs badStorage) Piece(p metainfo.Piece) storage.PieceImpl {
	return badStoragePiece{p}
}

type badStoragePiece struct {
	p metainfo.Piece
}

func (p badStoragePiece) WriteAt(b []byte, off int64) (int, error) {
	return 0, nil
}

func (p badStoragePiece) GetIsComplete() bool {
	return true
}

func (p badStoragePiece) MarkComplete() error {
	return errors.New("psyyyyyyyche")
}

func (p badStoragePiece) MarkNotComplete() error {
	return errors.New("psyyyyyyyche")
}

func (p badStoragePiece) randomlyTruncatedDataString() string {
	return "hello, world\n"[:rand.Intn(14)]
}

func (p badStoragePiece) ReadAt(b []byte, off int64) (n int, err error) {
	r := strings.NewReader(p.randomlyTruncatedDataString())
	return r.ReadAt(b, off+p.p.Offset())
}

// We read from a piece which is marked completed, but is missing data.
func TestCompletedPieceWrongSize(t *testing.T) {
	cfg := TestingConfig
	cfg.DefaultStorage = badStorage{}
	cl, err := NewClient(&cfg)
	require.NoError(t, err)
	defer cl.Close()
	info := metainfo.Info{
		PieceLength: 15,
		Pieces:      make([]byte, 20),
		Files: []metainfo.FileInfo{
			{Path: []string{"greeting"}, Length: 13},
		},
	}
	b, err := bencode.Marshal(info)
	tt, new, err := cl.AddTorrentSpec(&TorrentSpec{
		InfoBytes: b,
		InfoHash:  metainfo.HashBytes(b),
	})
	require.NoError(t, err)
	defer tt.Drop()
	assert.True(t, new)
	r := tt.NewReader()
	defer r.Close()
	b, err = ioutil.ReadAll(r)
	assert.Len(t, b, 13)
	assert.NoError(t, err)
}

func BenchmarkAddLargeTorrent(b *testing.B) {
	cfg := TestingConfig
	cfg.DisableTCP = true
	cfg.DisableUTP = true
	cfg.ListenAddr = "redonk"
	cl, err := NewClient(&cfg)
	require.NoError(b, err)
	defer cl.Close()
	for range iter.N(b.N) {
		t, err := cl.AddTorrentFromFile("testdata/bootstrap.dat.torrent")
		if err != nil {
			b.Fatal(err)
		}
		t.Drop()
	}
}

func TestResponsive(t *testing.T) {
	seederDataDir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(seederDataDir)
	cfg := TestingConfig
	cfg.Seed = true
	cfg.DataDir = seederDataDir
	seeder, err := NewClient(&cfg)
	require.Nil(t, err)
	defer seeder.Close()
	seeder.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	leecherDataDir, err := ioutil.TempDir("", "")
	require.Nil(t, err)
	defer os.RemoveAll(leecherDataDir)
	cfg = TestingConfig
	cfg.DataDir = leecherDataDir
	leecher, err := NewClient(&cfg)
	require.Nil(t, err)
	defer leecher.Close()
	leecherTorrent, _, _ := leecher.AddTorrentSpec(func() (ret *TorrentSpec) {
		ret = TorrentSpecFromMetaInfo(mi)
		ret.ChunkSize = 2
		return
	}())
	addClientPeer(leecherTorrent, seeder)
	reader := leecherTorrent.NewReader()
	defer reader.Close()
	reader.SetReadahead(0)
	reader.SetResponsive()
	b := make([]byte, 2)
	_, err = reader.Seek(3, os.SEEK_SET)
	require.NoError(t, err)
	_, err = io.ReadFull(reader, b)
	assert.Nil(t, err)
	assert.EqualValues(t, "lo", string(b))
	_, err = reader.Seek(11, os.SEEK_SET)
	require.NoError(t, err)
	n, err := io.ReadFull(reader, b)
	assert.Nil(t, err)
	assert.EqualValues(t, 2, n)
	assert.EqualValues(t, "d\n", string(b))
}

func TestTorrentDroppedDuringResponsiveRead(t *testing.T) {
	seederDataDir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(seederDataDir)
	cfg := TestingConfig
	cfg.Seed = true
	cfg.DataDir = seederDataDir
	seeder, err := NewClient(&cfg)
	require.Nil(t, err)
	defer seeder.Close()
	seeder.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	leecherDataDir, err := ioutil.TempDir("", "")
	require.Nil(t, err)
	defer os.RemoveAll(leecherDataDir)
	cfg = TestingConfig
	cfg.DataDir = leecherDataDir
	leecher, err := NewClient(&cfg)
	require.Nil(t, err)
	defer leecher.Close()
	leecherTorrent, _, _ := leecher.AddTorrentSpec(func() (ret *TorrentSpec) {
		ret = TorrentSpecFromMetaInfo(mi)
		ret.ChunkSize = 2
		return
	}())
	addClientPeer(leecherTorrent, seeder)
	reader := leecherTorrent.NewReader()
	defer reader.Close()
	reader.SetReadahead(0)
	reader.SetResponsive()
	b := make([]byte, 2)
	_, err = reader.Seek(3, os.SEEK_SET)
	require.NoError(t, err)
	_, err = io.ReadFull(reader, b)
	assert.Nil(t, err)
	assert.EqualValues(t, "lo", string(b))
	go leecherTorrent.Drop()
	_, err = reader.Seek(11, os.SEEK_SET)
	require.NoError(t, err)
	n, err := reader.Read(b)
	assert.EqualError(t, err, "torrent closed")
	assert.EqualValues(t, 0, n)
}

func TestDHTInheritBlocklist(t *testing.T) {
	ipl := iplist.New(nil)
	require.NotNil(t, ipl)
	cfg := TestingConfig
	cfg.IPBlocklist = ipl
	cfg.NoDHT = false
	cl, err := NewClient(&cfg)
	require.NoError(t, err)
	defer cl.Close()
	require.Equal(t, ipl, cl.DHT().IPBlocklist())
}

// Check that stuff is merged in subsequent AddTorrentSpec for the same
// infohash.
func TestAddTorrentSpecMerging(t *testing.T) {
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	defer cl.Close()
	dir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(dir)
	tt, new, err := cl.AddTorrentSpec(&TorrentSpec{
		InfoHash: mi.HashInfoBytes(),
	})
	require.NoError(t, err)
	require.True(t, new)
	require.Nil(t, tt.Info())
	_, new, err = cl.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	require.NoError(t, err)
	require.False(t, new)
	require.NotNil(t, tt.Info())
}

func TestTorrentDroppedBeforeGotInfo(t *testing.T) {
	dir, mi := testutil.GreetingTestTorrent()
	os.RemoveAll(dir)
	cl, _ := NewClient(&TestingConfig)
	defer cl.Close()
	tt, _, _ := cl.AddTorrentSpec(&TorrentSpec{
		InfoHash: mi.HashInfoBytes(),
	})
	tt.Drop()
	assert.EqualValues(t, 0, len(cl.Torrents()))
	select {
	case <-tt.GotInfo():
		t.FailNow()
	default:
	}
}

func writeTorrentData(ts *storage.Torrent, info metainfo.Info, b []byte) {
	for i := range iter.N(info.NumPieces()) {
		p := info.Piece(i)
		ts.Piece(p).WriteAt(b[p.Offset():p.Offset()+p.Length()], 0)
	}
}

func testAddTorrentPriorPieceCompletion(t *testing.T, alreadyCompleted bool, csf func(*filecache.Cache) storage.ClientImpl) {
	fileCacheDir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(fileCacheDir)
	fileCache, err := filecache.NewCache(fileCacheDir)
	require.NoError(t, err)
	greetingDataTempDir, greetingMetainfo := testutil.GreetingTestTorrent()
	defer os.RemoveAll(greetingDataTempDir)
	filePieceStore := csf(fileCache)
	info, err := greetingMetainfo.UnmarshalInfo()
	require.NoError(t, err)
	ih := greetingMetainfo.HashInfoBytes()
	greetingData, err := storage.NewClient(filePieceStore).OpenTorrent(&info, ih)
	require.NoError(t, err)
	writeTorrentData(greetingData, info, []byte(testutil.GreetingFileContents))
	// require.Equal(t, len(testutil.GreetingFileContents), written)
	// require.NoError(t, err)
	for i := 0; i < info.NumPieces(); i++ {
		p := info.Piece(i)
		if alreadyCompleted {
			err := greetingData.Piece(p).MarkComplete()
			assert.NoError(t, err)
		}
	}
	cfg := TestingConfig
	// TODO: Disable network option?
	cfg.DisableTCP = true
	cfg.DisableUTP = true
	cfg.DefaultStorage = filePieceStore
	cl, err := NewClient(&cfg)
	require.NoError(t, err)
	defer cl.Close()
	tt, err := cl.AddTorrent(greetingMetainfo)
	require.NoError(t, err)
	psrs := tt.PieceStateRuns()
	assert.Len(t, psrs, 1)
	assert.EqualValues(t, 3, psrs[0].Length)
	assert.Equal(t, alreadyCompleted, psrs[0].Complete)
	if alreadyCompleted {
		r := tt.NewReader()
		b, err := ioutil.ReadAll(r)
		assert.NoError(t, err)
		assert.EqualValues(t, testutil.GreetingFileContents, b)
	}
}

func TestAddTorrentPiecesAlreadyCompleted(t *testing.T) {
	testAddTorrentPriorPieceCompletion(t, true, fileCachePieceResourceStorage)
}

func TestAddTorrentPiecesNotAlreadyCompleted(t *testing.T) {
	testAddTorrentPriorPieceCompletion(t, false, fileCachePieceResourceStorage)
}

func TestAddMetainfoWithNodes(t *testing.T) {
	cfg := TestingConfig
	cfg.NoDHT = false
	// For now, we want to just jam the nodes into the table, without
	// verifying them first. Also the DHT code doesn't support mixing secure
	// and insecure nodes if security is enabled (yet).
	cfg.DHTConfig.NoSecurity = true
	cl, err := NewClient(&cfg)
	require.NoError(t, err)
	defer cl.Close()
	assert.EqualValues(t, cl.DHT().NumNodes(), 0)
	tt, err := cl.AddTorrentFromFile("metainfo/testdata/issue_65a.torrent")
	require.NoError(t, err)
	assert.Len(t, tt.metainfo.AnnounceList, 5)
	assert.EqualValues(t, 6, cl.DHT().NumNodes())
}

type testDownloadCancelParams struct {
	ExportClientStatus        bool
	SetLeecherStorageCapacity bool
	LeecherStorageCapacity    int64
	Cancel                    bool
}

func testDownloadCancel(t *testing.T, ps testDownloadCancelParams) {
	greetingTempDir, mi := testutil.GreetingTestTorrent()
	defer os.RemoveAll(greetingTempDir)
	cfg := TestingConfig
	cfg.Seed = true
	cfg.DataDir = greetingTempDir
	seeder, err := NewClient(&cfg)
	require.NoError(t, err)
	defer seeder.Close()
	if ps.ExportClientStatus {
		testutil.ExportStatusWriter(seeder, "s")
	}
	seeder.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	leecherDataDir, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(leecherDataDir)
	fc, err := filecache.NewCache(leecherDataDir)
	require.NoError(t, err)
	if ps.SetLeecherStorageCapacity {
		fc.SetCapacity(ps.LeecherStorageCapacity)
	}
	cfg.DefaultStorage = storage.NewResourcePieces(fc.AsResourceProvider())
	cfg.DataDir = leecherDataDir
	leecher, _ := NewClient(&cfg)
	defer leecher.Close()
	if ps.ExportClientStatus {
		testutil.ExportStatusWriter(leecher, "l")
	}
	leecherGreeting, new, err := leecher.AddTorrentSpec(func() (ret *TorrentSpec) {
		ret = TorrentSpecFromMetaInfo(mi)
		ret.ChunkSize = 2
		return
	}())
	require.NoError(t, err)
	assert.True(t, new)
	psc := leecherGreeting.SubscribePieceStateChanges()
	defer psc.Close()
	leecherGreeting.DownloadAll()
	if ps.Cancel {
		leecherGreeting.CancelPieces(0, leecherGreeting.NumPieces())
	}
	addClientPeer(leecherGreeting, seeder)
	completes := make(map[int]bool, 3)
values:
	for {
		// started := time.Now()
		select {
		case _v := <-psc.Values:
			// log.Print(time.Since(started))
			v := _v.(PieceStateChange)
			completes[v.Index] = v.Complete
		case <-time.After(100 * time.Millisecond):
			break values
		}
	}
	if ps.Cancel {
		assert.EqualValues(t, map[int]bool{0: false, 1: false, 2: false}, completes)
	} else {
		assert.EqualValues(t, map[int]bool{0: true, 1: true, 2: true}, completes)
	}

}

func TestTorrentDownloadAll(t *testing.T) {
	testDownloadCancel(t, testDownloadCancelParams{})
}

func TestTorrentDownloadAllThenCancel(t *testing.T) {
	testDownloadCancel(t, testDownloadCancelParams{
		Cancel: true,
	})
}

// Ensure that it's an error for a peer to send an invalid have message.
func TestPeerInvalidHave(t *testing.T) {
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	defer cl.Close()
	info := metainfo.Info{
		PieceLength: 1,
		Pieces:      make([]byte, 20),
		Files:       []metainfo.FileInfo{{Length: 1}},
	}
	infoBytes, err := bencode.Marshal(info)
	require.NoError(t, err)
	tt, _new, err := cl.AddTorrentSpec(&TorrentSpec{
		InfoBytes: infoBytes,
		InfoHash:  metainfo.HashBytes(infoBytes),
	})
	require.NoError(t, err)
	assert.True(t, _new)
	defer tt.Drop()
	cn := &connection{
		t: tt,
	}
	assert.NoError(t, cn.peerSentHave(0))
	assert.Error(t, cn.peerSentHave(1))
}

func TestPieceCompletedInStorageButNotClient(t *testing.T) {
	greetingTempDir, greetingMetainfo := testutil.GreetingTestTorrent()
	defer os.RemoveAll(greetingTempDir)
	cfg := TestingConfig
	cfg.DataDir = greetingTempDir
	seeder, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	seeder.AddTorrentSpec(&TorrentSpec{
		InfoBytes: greetingMetainfo.InfoBytes,
	})
}

func TestPrepareTrackerAnnounce(t *testing.T) {
	cl := &Client{}
	blocked, urlToUse, host, err := cl.prepareTrackerAnnounceUnlocked("http://localhost:1234/announce?herp")
	require.NoError(t, err)
	assert.False(t, blocked)
	assert.EqualValues(t, "localhost:1234", host)
	assert.EqualValues(t, "http://127.0.0.1:1234/announce?herp", urlToUse)
}

// Check that when the listen port is 0, all the protocols listened on have
// the same port, and it isn't zero.
func TestClientDynamicListenPortAllProtocols(t *testing.T) {
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	defer cl.Close()
	assert.NotEqual(t, 0, missinggo.AddrPort(cl.ListenAddr()))
	assert.Equal(t, missinggo.AddrPort(cl.utpSock.Addr()), missinggo.AddrPort(cl.tcpListener.Addr()))
}

func TestClientDynamicListenTCPOnly(t *testing.T) {
	cfg := TestingConfig
	cfg.DisableUTP = true
	cl, err := NewClient(&cfg)
	require.NoError(t, err)
	defer cl.Close()
	assert.NotEqual(t, 0, missinggo.AddrPort(cl.ListenAddr()))
	assert.Nil(t, cl.utpSock)
}

func TestClientDynamicListenUTPOnly(t *testing.T) {
	cfg := TestingConfig
	cfg.DisableTCP = true
	cl, err := NewClient(&cfg)
	require.NoError(t, err)
	defer cl.Close()
	assert.NotEqual(t, 0, missinggo.AddrPort(cl.ListenAddr()))
	assert.Nil(t, cl.tcpListener)
}

func TestClientDynamicListenPortNoProtocols(t *testing.T) {
	cfg := TestingConfig
	cfg.DisableTCP = true
	cfg.DisableUTP = true
	cl, err := NewClient(&cfg)
	require.NoError(t, err)
	defer cl.Close()
	assert.Nil(t, cl.ListenAddr())
}

func addClientPeer(t *Torrent, cl *Client) {
	t.AddPeers([]Peer{
		{
			IP:   missinggo.AddrIP(cl.ListenAddr()),
			Port: missinggo.AddrPort(cl.ListenAddr()),
		},
	})
}

func printConnPeerCounts(t *Torrent) {
	t.cl.mu.Lock()
	log.Println(len(t.conns), len(t.peers))
	t.cl.mu.Unlock()
}

func totalConns(tts []*Torrent) (ret int) {
	for _, tt := range tts {
		tt.cl.mu.Lock()
		ret += len(tt.conns)
		tt.cl.mu.Unlock()
	}
	return
}

func TestSetMaxEstablishedConn(t *testing.T) {
	var tts []*Torrent
	ih := testutil.GreetingMetaInfo().HashInfoBytes()
	cfg := TestingConfig
	for i := range iter.N(3) {
		cl, err := NewClient(&cfg)
		require.NoError(t, err)
		defer cl.Close()
		tt, _ := cl.AddTorrentInfoHash(ih)
		tt.SetMaxEstablishedConns(2)
		testutil.ExportStatusWriter(cl, fmt.Sprintf("%d", i))
		tts = append(tts, tt)
	}
	addPeers := func() {
		for i, tt := range tts {
			for _, _tt := range tts[:i] {
				addClientPeer(tt, _tt.cl)
			}
		}
	}
	waitTotalConns := func(num int) {
		for totalConns(tts) != num {
			time.Sleep(time.Millisecond)
		}
	}
	addPeers()
	waitTotalConns(6)
	tts[0].SetMaxEstablishedConns(1)
	waitTotalConns(4)
	tts[0].SetMaxEstablishedConns(0)
	waitTotalConns(2)
	tts[0].SetMaxEstablishedConns(1)
	addPeers()
	waitTotalConns(4)
	tts[0].SetMaxEstablishedConns(2)
	addPeers()
	waitTotalConns(6)
}

func makeMagnet(t *testing.T, cl *Client, dir string, name string) string {
	os.MkdirAll(dir, 0770)
	file, err := os.Create(filepath.Join(dir, name))
	require.NoError(t, err)
	file.Write([]byte(name))
	file.Close()
	mi := metainfo.MetaInfo{}
	mi.SetDefaults()
	info := metainfo.Info{PieceLength: 256 * 1024}
	err = info.BuildFromFilePath(filepath.Join(dir, name))
	require.NoError(t, err)
	mi.InfoBytes, err = bencode.Marshal(info)
	require.NoError(t, err)
	magnet := mi.Magnet(name, mi.HashInfoBytes()).String()
	tr, err := cl.AddTorrent(&mi)
	require.NoError(t, err)
	assert.True(t, tr.Seeding())
	return magnet
}

// https://github.com/anacrolix/torrent/issues/114
func TestMultipleTorrentsWithEncryption(t *testing.T) {
	cfg := TestingConfig
	cfg.DisableUTP = true
	cfg.Seed = true
	cfg.DataDir = filepath.Join(cfg.DataDir, "server")
	cfg.Debug = true
	cfg.ForceEncryption = true
	os.Mkdir(cfg.DataDir, 0755)
	server, err := NewClient(&cfg)
	require.NoError(t, err)
	defer server.Close()
	testutil.ExportStatusWriter(server, "s")
	magnet1 := makeMagnet(t, server, cfg.DataDir, "test1")
	makeMagnet(t, server, cfg.DataDir, "test2")
	cfg = TestingConfig
	cfg.DisableUTP = true
	cfg.DataDir = filepath.Join(cfg.DataDir, "client")
	cfg.Debug = true
	cfg.ForceEncryption = true
	client, err := NewClient(&cfg)
	require.NoError(t, err)
	defer client.Close()
	testutil.ExportStatusWriter(client, "c")
	tr, err := client.AddMagnet(magnet1)
	require.NoError(t, err)
	tr.AddPeers([]Peer{{
		IP:   missinggo.AddrIP(server.ListenAddr()),
		Port: missinggo.AddrPort(server.ListenAddr()),
	}})
	<-tr.GotInfo()
	tr.DownloadAll()
	client.WaitAll()
}

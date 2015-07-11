package native

import (
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/jpillora/cloud-torrent/ct/shared"
)

//the native Cloud Torrent engine, backed by anacrolix/torrent
type Native struct {
	config *config
	client *torrent.Client
	queue  chan *shared.Torrent
}

func (n *Native) Name() string {
	return "Native"
}
func (n *Native) NewConfig() interface{} {
	//default config
	return &config{
		Config: torrent.Config{
			//apply defaults
			DataDir: ".",
		},
	}
}

func (n *Native) SetConfig(obj interface{}) error {
	//recieve config
	c, ok := obj.(*config)
	if !ok {
		return fmt.Errorf("Invalid config")
	}

	client, err := torrent.NewClient(&c.Config)
	if err != nil {
		return err
	}
	n.client = client
	return nil
}

func (n *Native) NewTorrent(magnetURI string) error {
	_, err := n.client.AddMagnet(magnetURI)
	if err != nil {
		return err
	}
	return nil
}

func (n *Native) getTorrent(infohash string) (torrent.Torrent, error) {
	var t torrent.Torrent
	ih, err := str2ih(infohash)
	if err != nil {
		return t, err
	}
	t, ok := n.client.Torrent(ih)
	if !ok {
		return t, fmt.Errorf("Missing torrent %x", ih)
	}
	return t, nil
}

func (n *Native) StartTorrent(infohash string) error {
	log.Printf("start %s", infohash)
	t, err := n.getTorrent(infohash)
	if err != nil {
		return err
	}
	t.DownloadAll()
	return nil
}

func (n *Native) StopTorrent(infohash string) error {
	return fmt.Errorf("Unsupported")
}

func (n *Native) DeleteTorrent(infohash string) error {
	t, err := n.getTorrent(infohash)
	if err != nil {
		return err
	}
	t.Drop()
	return nil
}

func (n *Native) getFile(infohash, filepath string) (file torrent.File, err error) {
	t, err := n.getTorrent(infohash)
	if err != nil {
		return
	}
	for _, f := range t.Files() {
		if filepath == f.Path() {
			file = f
			return
		}
	}
	err = fmt.Errorf("File not found")
	return
}

func (n *Native) StartFile(infohash, filepath string) error {
	f, err := n.getFile(infohash, filepath)
	if err != nil {
		return err
	}
	f.PrioritizeRegion(0, f.Length())
	return nil
}

func (n *Native) StopFile(infohash, filepath string) error {
	return fmt.Errorf("Unsupported")
}

func (n *Native) GetTorrents() <-chan *shared.Torrent {
	n.queue = make(chan *shared.Torrent)
	go n.pollTorrents()
	return n.queue
}

func (n *Native) pollTorrents() {
	for {
		time.Sleep(time.Second)
		if n.client == nil {
			continue
		}
		for _, t := range n.client.Torrents() {
			//copy torrent info
			st := &shared.Torrent{
				Name:     t.Name(),
				Loaded:   t.Info() != nil,
				InfoHash: t.InfoHash.HexString(),
				Progress: t.BytesCompleted(),
				Size:     t.Length(),
			}
			//copy files info
			files := t.Files()
			st.Files = make([]*shared.File, len(files))
			for i, f1 := range files {
				peices := f1.State()
				f2 := &shared.File{
					Path:      f1.Path(),
					Size:      f1.Length(),
					Chunks:    len(peices),
					Completed: 0,
				}
				for _, p := range peices {
					if p.Complete {
						f2.Completed++
					}
				}
				st.Files[i] = f2
			}
			//enqueue update
			n.queue <- st
		}
	}
}

func str2ih(str string) (torrent.InfoHash, error) {
	var ih torrent.InfoHash
	n, err := hex.Decode(ih[:], []byte(str))
	if err != nil {
		return ih, fmt.Errorf("Invalid hex string")
	}
	if n != 20 {
		return ih, fmt.Errorf("Invalid length")
	}
	return ih, nil
}

//mask over TorrentDataOpener to allow torrent.Config to be marshalled
type config struct {
	torrent.Config
	TorrentDataOpener string `json:",omitempty"` //masks func
}

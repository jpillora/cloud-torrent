package native

import (
	"fmt"
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

func (n *Native) Magnet(uri string) error {
	_, err := n.client.AddMagnet(uri)
	if err != nil {
		return err
	}
	return nil
}

func (n *Native) Torrents() <-chan *shared.Torrent {
	n.queue = make(chan *shared.Torrent)
	go n.pollTorrents()
	return n.queue
}

func (n *Native) pollTorrents() {
	for {
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
				peices := f1.Progress()
				f2 := &shared.File{
					Path:      f1.Path(),
					Size:      f1.Length(),
					Chunks:    len(peices),
					Completed: 0,
				}
				for _, p := range peices {
					if p.State == 'C' {
						f2.Completed++
					}
				}
				st.Files[i] = f2
			}
			//enqueue update
			n.queue <- st
		}
		time.Sleep(time.Second)
	}
}

//mask over TorrentDataOpener to allow torrent.Config to be parsed
type config struct {
	torrent.Config
	TorrentDataOpener string `json:",omitempty"` //masks func
}

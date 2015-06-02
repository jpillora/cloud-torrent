package native

import (
	"encoding/hex"
	"fmt"

	"github.com/anacrolix/torrent"
	"github.com/jpillora/cloud-torrent/ct/shared"
)

//trick json into allowing torrent.Config to be parsed
type config struct {
	torrent.Config
	TorrentDataOpener string `json:",omitempty"` //masks func
}

//the native Cloud Torrent engine, backed by anacrolix/torrent
type Native struct {
	config   *config
	client   *torrent.Client
	torrents []*torrent.Torrent
}

func (n *Native) Name() string {
	return "Native"
}
func (n *Native) GetConfig() interface{} {
	config := &config{
		Config: torrent.Config{
			//apply defaults
			DataDir: ".",
		},
	}
	n.config = config
	return config
}

func (n *Native) SetConfig() error {
	c, err := torrent.NewClient(&n.config.Config)
	if err != nil {
		return err
	}
	n.client = c
	return nil
}

func (n *Native) Magnet(uri string) error {
	_, err := n.client.AddMagnet(uri)
	if err != nil {
		return err
	}
	return nil
}

func (n *Native) List() ([]*shared.Torrent, error) {
	t1 := n.client.Torrents()
	t2 := make([]*shared.Torrent, len(t1))
	for i, t := range t1 {
		t2[i] = &shared.Torrent{
			Name:     t.Name(),
			Progress: t.BytesCompleted(),
			Size:     t.Length(),
		}
	}
	return t2, nil
}

func (n *Native) Fetch(t2 *shared.Torrent) error {

	var ih torrent.InfoHash
	if _, err := hex.Decode(ih[:], []byte(t2.InfoHash)); err != nil {
		return err
	}

	t1, ok := n.client.Torrent(ih)
	if !ok {
		return fmt.Errorf("Torrent not found: %s", t2.InfoHash)
	}

	t1fs := t1.Files()
	t2.Files = make([]*shared.File, len(t1fs))
	for i, f1 := range t1fs {
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
		t2.Files[i] = f2
	}

	return nil
}

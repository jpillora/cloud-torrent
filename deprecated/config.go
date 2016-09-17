package server

import "github.com/jpillora/cloud-torrent/engine"
import "github.com/jpillora/cloud-torrent/storage"

type Config struct {
	Torrent engine.Config   `json:"torrent"`
	Storage storage.Configs `json:"storage"`
}

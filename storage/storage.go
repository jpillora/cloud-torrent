package storage

import (
	"log"

	"github.com/spf13/afero"
)

type Configs struct {
	Disk *AferoConfig `json:"disk"`
}

type Storage struct {
	FileLimit   int
	FileSystems map[string]Fs
}

func New() *Storage {
	s := &Storage{
		FileSystems: map[string]Fs{
			"disk":   newAferoFs(afero.NewOsFs()),
			"memory": newAferoFs(afero.NewMemMapFs()),
		},
	}
	return s
}

func (s *Storage) Get(id string) (Fs, bool) {
	fs, ok := s.FileSystems[id]
	return fs, ok
}

func (s *Storage) Configure(c Configs) error {
	if c.Disk != nil {
		log.Printf("conf disk")
		if err := s.FileSystems["disk"].Configure(&c.Disk); err != nil {
			return err
		}
	}
	return nil
}

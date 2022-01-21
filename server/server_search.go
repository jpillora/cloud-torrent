package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
)

//go:embed default-scraper-config.json
var defaultSearchConfig []byte
var currentConfig []byte

func (s *Server) fetchSearchConfig(confurl string) error {
	if !strings.HasPrefix(confurl, "http") {
		log.Println("fetchSearchConfig: unconfigured, using the default conf", confurl)
		return nil
	}
	log.Println("fetchSearchConfig: loading search config from", confurl)
	resp, err := http.Get(confurl)
	if err != nil {
		log.Println("[fetchSearchConfig]", err)
		return err
	}
	defer resp.Body.Close()
	newConfig, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	newConfig, err = normalize(newConfig)
	if err != nil {
		return err
	}
	if bytes.Equal(currentConfig, newConfig) {
		return nil //skip
	}
	if err := s.scraper.LoadConfig(newConfig); err != nil {
		return err
	}
	s.searchProviders = &s.scraper.Config
	currentConfig = newConfig
	log.Printf("Loaded new search providers")
	return nil
}

func normalize(input []byte) ([]byte, error) {
	output := bytes.Buffer{}
	if err := json.Indent(&output, input, "", "  "); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

//see github.com/jpillora/scraper for config specification
//cloud-torrent uses "<id>-item" handlers

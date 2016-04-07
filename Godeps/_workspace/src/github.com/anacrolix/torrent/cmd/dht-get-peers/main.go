package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"

	_ "github.com/anacrolix/envpprof"

	"github.com/anacrolix/torrent/dht"
)

var (
	tableFileName = flag.String("tableFile", "", "name of file for storing node info")
	serveAddr     = flag.String("serveAddr", ":0", "local UDP address")
	infoHash      = flag.String("infoHash", "", "torrent infohash")
	once          = flag.Bool("once", false, "only do one scrape iteration")

	s        *dht.Server
	quitting = make(chan struct{})
)

func loadTable() error {
	if *tableFileName == "" {
		return nil
	}
	f, err := os.Open(*tableFileName)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error opening table file: %s", err)
	}
	defer f.Close()
	added := 0
	for {
		b := make([]byte, dht.CompactIPv4NodeInfoLen)
		_, err := io.ReadFull(f, b)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading table file: %s", err)
		}
		var ni dht.NodeInfo
		err = ni.UnmarshalCompactIPv4(b)
		if err != nil {
			return fmt.Errorf("error unmarshaling compact node info: %s", err)
		}
		s.AddNode(ni)
		added++
	}
	log.Printf("loaded %d nodes from table file", added)
	return nil
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
	switch len(*infoHash) {
	case 20:
	case 40:
		_, err := fmt.Sscanf(*infoHash, "%x", infoHash)
		if err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("require 20 byte infohash")
	}
	var err error
	s, err = dht.NewServer(&dht.ServerConfig{
		Addr: *serveAddr,
	})
	if err != nil {
		log.Fatal(err)
	}
	err = loadTable()
	if err != nil {
		log.Fatalf("error loading table: %s", err)
	}
	log.Printf("dht server on %s, ID is %x", s.Addr(), s.ID())
	setupSignals()
}

func saveTable() error {
	goodNodes := s.Nodes()
	if *tableFileName == "" {
		if len(goodNodes) != 0 {
			log.Print("good nodes were discarded because you didn't specify a table file")
		}
		return nil
	}
	f, err := os.OpenFile(*tableFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("error opening table file: %s", err)
	}
	defer f.Close()
	for _, nodeInfo := range goodNodes {
		var b [dht.CompactIPv4NodeInfoLen]byte
		err := nodeInfo.PutCompact(b[:])
		if err != nil {
			return fmt.Errorf("error compacting node info: %s", err)
		}
		_, err = f.Write(b[:])
		if err != nil {
			return fmt.Errorf("error writing compact node info: %s", err)
		}
	}
	log.Printf("saved %d nodes to table file", len(goodNodes))
	return nil
}

func setupSignals() {
	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		close(quitting)
	}()
}

func main() {
	seen := make(map[string]struct{})
getPeers:
	for {
		ps, err := s.Announce(*infoHash, 0, false)
		if err != nil {
			log.Fatal(err)
		}
	values:
		for {
			select {
			case v, ok := <-ps.Peers:
				if !ok {
					break values
				}
				log.Printf("received %d peers from %x", len(v.Peers), v.NodeInfo.ID)
				for _, p := range v.Peers {
					if _, ok := seen[p.String()]; ok {
						continue
					}
					seen[p.String()] = struct{}{}
					fmt.Println((&net.UDPAddr{
						IP:   p.IP[:],
						Port: int(p.Port),
					}).String())
				}
			case <-quitting:
				break getPeers
			}
		}
		if *once {
			break
		}
	}
	if err := saveTable(); err != nil {
		log.Printf("error saving node table: %s", err)
	}
}

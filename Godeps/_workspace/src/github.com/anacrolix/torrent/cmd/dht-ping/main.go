// Pings DHT nodes with the given network addresses.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/anacrolix/torrent/dht"
)

type pingResponse struct {
	addr string
	krpc dht.Msg
	rtt  time.Duration
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	timeout := flag.Duration("timeout", -1, "maximum timeout")
	flag.Parse()
	pingStrAddrs := flag.Args()
	if len(pingStrAddrs) == 0 {
		os.Stderr.WriteString("u must specify addrs of nodes to ping e.g. router.bittorrent.com:6881\n")
		os.Exit(2)
	}
	s, err := dht.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("dht server on %s", s.Addr())
	pingResponses := make(chan pingResponse)
	timeoutChan := make(chan struct{})
	go func() {
		for i, netloc := range pingStrAddrs {
			if i != 0 {
				time.Sleep(1 * time.Millisecond)
			}
			addr, err := net.ResolveUDPAddr("udp4", netloc)
			if err != nil {
				log.Fatal(err)
			}
			t, err := s.Ping(addr)
			if err != nil {
				log.Fatal(err)
			}
			start := time.Now()
			t.SetResponseHandler(func(addr string) func(dht.Msg) {
				return func(resp dht.Msg) {
					pingResponses <- pingResponse{
						addr: addr,
						krpc: resp,
						rtt:  time.Now().Sub(start),
					}
				}
			}(netloc))
		}
		if *timeout >= 0 {
			time.Sleep(*timeout)
			close(timeoutChan)
		}
	}()
	responses := 0
pingResponses:
	for _ = range pingStrAddrs {
		select {
		case resp := <-pingResponses:
			if resp.krpc == nil {
				break
			}
			responses++
			fmt.Printf("%-65s %s\n", fmt.Sprintf("%x (%s):", resp.krpc["r"].(map[string]interface{})["id"].(string), resp.addr), resp.rtt)
		case <-timeoutChan:
			break pingResponses
		}
	}
	// timeouts := len(pingStrAddrs) - responses
	fmt.Printf("%d/%d responses (%f%%)\n", responses, len(pingStrAddrs), 100*float64(responses)/float64(len(pingStrAddrs)))
}

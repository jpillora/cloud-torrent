package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"

	"github.com/docopt/docopt-go"

	"github.com/anacrolix/torrent/dht"
)

func main() {
	args, _ := docopt.Parse(`dht-secure-id outputs the node ID secured with the IP.

Usage: dht-secure-id <id> <ip>`, nil, true, "", false)
	id, err := hex.DecodeString(args["<id>"].(string))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad id: %s\n", err)
		os.Exit(2)
	}
	if len(id) != 20 {
		fmt.Fprintf(os.Stderr, "bad id: wrong length\n")
		os.Exit(2)
	}
	ip := net.ParseIP(args["<ip>"].(string))
	if ip == nil {
		fmt.Fprintf(os.Stderr, "bad ip\n")
		os.Exit(2)
	}
	dht.SecureNodeId(id, ip)
	fmt.Printf("%x\n", id)
}

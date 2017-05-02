package main

import (
	"log"
	"net/http"

	"github.com/jpillora/sizestr"
)

//go client for performance testing

func main() {
	req, _ := http.NewRequest("GET", "http://localhost:7070/sync", nil)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("request: %s", err)
	}
	r := resp.Body

	i := 0
	buff := make([]byte, 32*1024)
	for {
		n, err := r.Read(buff)
		if err != nil {
			break
		}
		i++
		log.Printf("#%d: %s", i, sizestr.ToString(int64(n)))
	}

	r.Close()
	log.Printf("closed")
}

type InlineWriter struct {
	fn func(p []byte) (int, error)
}

func (i *InlineWriter) Write(p []byte) (int, error) {
	return i.fn(p)
}

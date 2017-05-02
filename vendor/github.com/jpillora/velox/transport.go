package velox

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bernerdschaefer/eventsource"
	"github.com/gorilla/websocket"
)

//a single update
type update struct {
	ID      string          `json:"id,omitempty"`
	Ping    bool            `json:"ping,omitempty"`
	Delta   bool            `json:"delta,omitempty"`
	Version int64           `json:"version,omitempty"` //53 usable bits
	Body    json.RawMessage `json:"body,omitempty"`
}

type transport interface {
	connect(w http.ResponseWriter, r *http.Request) error
	send(upd *update) error
	wait() error
	close() error
}

//=========================

var defaultUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type websocketsTransport struct {
	writeTimeout time.Duration
	conn         *websocket.Conn
}

func (ws *websocketsTransport) connect(w http.ResponseWriter, r *http.Request) error {
	conn, err := defaultUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("[velox] cannot upgrade connection: %s", err)
	}
	ws.conn = conn
	return nil
}

func (ws *websocketsTransport) send(upd *update) error {
	ws.conn.SetWriteDeadline(time.Now().Add(ws.writeTimeout))
	return ws.conn.WriteJSON(upd)
}

func (ws *websocketsTransport) wait() error {
	//block on connection
	for {
		//ws is bi-directional, so we can rely on pings
		//from clients. currently hardcoded to 25s so timeout
		//after 30s.
		ws.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		if _, _, err := ws.conn.ReadMessage(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
func (ws *websocketsTransport) close() error {
	return ws.conn.Close()
}

//=========================
//eventsource is implemented over raw tcp
//to allow for control over timeouts.
//the message pipeline is as follows:
// 1. raw message
// 2. eventsource message
// 3. chunked encoded
// 4. gzip [optional]
// 5. buffered connection

type eventSourceTransport struct {
	writeTimeout time.Duration
	conn         net.Conn
	rw           *bufio.ReadWriter
	gw           *gzip.Writer
	enc          *eventsource.Encoder
	chunked      io.WriteCloser
	dst          io.Writer
	mut          sync.Mutex
	ferr         error
	useGzip      bool
	isConnected  bool
	connected    chan struct{}
}

func (es *eventSourceTransport) connect(w http.ResponseWriter, r *http.Request) error {
	//hijack
	hj, ok := w.(http.Hijacker)
	if !ok {
		return errors.New("[velox] underlying writer must be an http.Hijacker")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return errors.New("[velox] failed to hijack underlying net.Conn")
	}
	//can we gzip?
	es.useGzip = strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	//init
	es.conn = conn
	es.rw = rw
	es.chunked = httputil.NewChunkedWriter(rw)
	if es.useGzip {
		es.gw = gzip.NewWriter(es.chunked)
		es.dst = es.gw
	} else {
		es.dst = es.chunked
	}
	//http and eventsource headers
	rw.WriteString("HTTP/1.1 200 OK\r\n")
	h := http.Header{}
	wh := w.Header()
	for k, _ := range wh {
		h.Set(k, wh.Get(k))
	}
	h.Set("Cache-Control", "no-cache")
	h.Set("Vary", "Accept")
	h.Set("Content-Type", "text/event-stream")
	if es.useGzip {
		h.Set("Content-Encoding", "gzip")
	} else {
		h.Del("Content-Encoding")
	}
	h.Write(rw)
	h = http.Header{}
	h.Set("Transfer-Encoding", "chunked")
	h.Write(rw)
	rw.WriteString("\r\n")
	rw.Flush()
	//connection is now expecting a chunked stream of events
	es.enc = eventsource.NewEncoder(es)
	//ready!
	return nil
}

func (es *eventSourceTransport) send(upd *update) error {
	b, err := json.Marshal(upd)
	if err != nil {
		return err
	}
	return es.enc.Encode(eventsource.Event{
		ID:   strconv.FormatInt(upd.Version, 10),
		Data: b,
	})
}

func (es *eventSourceTransport) wait() error {
	//disable readtime outs
	es.conn.SetReadDeadline(time.Time{})
	//read to /dev/null
	_, err := io.Copy(ioutil.Discard, es.rw)
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (es *eventSourceTransport) close() error {
	es.enc.Flush()
	es.chunked.Close()
	if es.useGzip {
		es.gw.Close()
	}
	es.rw.Flush()
	return es.conn.Close()
}

func (es *eventSourceTransport) Write(p []byte) (int, error) {
	es.mut.Lock()
	defer es.mut.Unlock()
	//propagate flush errors here
	if es.ferr != nil {
		return 0, es.ferr
	}
	//write to chunked/gzip (backed by buffered connection)
	return es.dst.Write(p)
}

//flush converts the buffer into chunked then does write
func (es *eventSourceTransport) Flush() {
	es.mut.Lock()
	defer es.mut.Unlock()
	//expect connection write within <write-timeout>
	es.conn.SetWriteDeadline(time.Now().Add(es.writeTimeout))
	//flush gzip and buffered connection
	if es.useGzip {
		es.gw.Flush()
	}
	es.ferr = es.rw.Flush()
}

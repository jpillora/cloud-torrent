package realtime

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type objectVersions map[key]int64 //maps object key -> version

type User struct {
	mut       sync.Mutex //protects all user fields
	Connected bool
	ID        string
	uptime    time.Time
	conn      *websocket.Conn
	versions  objectVersions
	pending   []*update
}

func (u *User) sendPending() {
	u.mut.Lock()
	if len(u.pending) > 0 {
		u.conn.WriteJSON(u.pending)
		u.pending = nil
	}
	u.mut.Unlock()
}

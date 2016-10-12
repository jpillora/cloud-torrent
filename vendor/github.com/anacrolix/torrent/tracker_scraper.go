package torrent

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/tracker"
)

// Announces a torrent to a tracker at regular intervals, when peers are
// required.
type trackerScraper struct {
	url string
	// Causes the trackerScraper to stop running.
	stop         missinggo.Event
	t            *Torrent
	lastAnnounce trackerAnnounceResult
}

func (ts *trackerScraper) statusLine() string {
	var w bytes.Buffer
	fmt.Fprintf(&w, "%q\t%s\t%s",
		ts.url,
		func() string {
			// return ts.lastAnnounce.Completed.Add(ts.lastAnnounce.Interval).Format("2006-01-02 15:04:05 -0700 MST")
			na := ts.lastAnnounce.Completed.Add(ts.lastAnnounce.Interval).Sub(time.Now())
			na /= time.Second
			na *= time.Second
			if na > 0 {
				return na.String()
			} else {
				return "anytime"
			}
		}(),
		func() string {
			if ts.lastAnnounce.Err != nil {
				return ts.lastAnnounce.Err.Error()
			}
			if ts.lastAnnounce.Completed.IsZero() {
				return "never"
			}
			return fmt.Sprintf("%d peers", ts.lastAnnounce.NumPeers)
		}())
	return w.String()
}

type trackerAnnounceResult struct {
	Err       error
	NumPeers  int
	Interval  time.Duration
	Completed time.Time
}

func trackerToTorrentPeers(ps []tracker.Peer) (ret []Peer) {
	ret = make([]Peer, 0, len(ps))
	for _, p := range ps {
		ret = append(ret, Peer{
			IP:     p.IP,
			Port:   p.Port,
			Source: peerSourceTracker,
		})
	}
	return
}

// Return how long to wait before trying again. For most errors, we return 5
// minutes, a relatively quick turn around for DNS changes.
func (me *trackerScraper) announce() (ret trackerAnnounceResult) {
	defer func() {
		ret.Completed = time.Now()
	}()
	ret.Interval = 5 * time.Minute
	blocked, urlToUse, host, err := me.t.cl.prepareTrackerAnnounceUnlocked(me.url)
	if err != nil {
		ret.Err = err
		return
	}
	if blocked {
		ret.Err = errors.New("blocked by IP")
		return
	}
	me.t.cl.mu.Lock()
	req := me.t.announceRequest()
	me.t.cl.mu.Unlock()
	res, err := tracker.AnnounceHost(urlToUse, &req, host)
	if err != nil {
		ret.Err = err
		return
	}
	me.t.AddPeers(trackerToTorrentPeers(res.Peers))
	ret.NumPeers = len(res.Peers)
	ret.Interval = time.Duration(res.Interval) * time.Second
	return
}

func (me *trackerScraper) Run() {
	for {
		select {
		case <-me.t.closed.LockedChan(&me.t.cl.mu):
			return
		case <-me.stop.LockedChan(&me.t.cl.mu):
			return
		case <-me.t.wantPeersEvent.LockedChan(&me.t.cl.mu):
		}

		ar := me.announce()
		me.t.cl.mu.Lock()
		me.lastAnnounce = ar
		me.t.cl.mu.Unlock()

		intervalChan := time.After(ar.Completed.Add(ar.Interval).Sub(time.Now()))

		select {
		case <-me.t.closed.LockedChan(&me.t.cl.mu):
			return
		case <-me.stop.LockedChan(&me.t.cl.mu):
			return
		case <-intervalChan:
		}
	}
}

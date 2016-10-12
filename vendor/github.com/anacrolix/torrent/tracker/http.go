package tracker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/anacrolix/missinggo/httptoo"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/util"
)

type httpResponse struct {
	FailureReason string      `bencode:"failure reason"`
	Interval      int32       `bencode:"interval"`
	TrackerId     string      `bencode:"tracker id"`
	Complete      int32       `bencode:"complete"`
	Incomplete    int32       `bencode:"incomplete"`
	Peers         interface{} `bencode:"peers"`
}

func (r *httpResponse) UnmarshalPeers() (ret []Peer, err error) {
	s, ok := r.Peers.(string)
	if !ok {
		err = fmt.Errorf("unsupported peers value type: %T", r.Peers)
		return
	}
	cp, err := util.UnmarshalIPv4CompactPeers([]byte(s))
	if err != nil {
		return
	}
	ret = make([]Peer, 0, len(cp))
	for _, p := range cp {
		ret = append(ret, Peer{net.IP(p.IP[:]), int(p.Port)})
	}
	return
}

func setAnnounceParams(_url *url.URL, ar *AnnounceRequest) {
	q := _url.Query()

	q.Set("info_hash", string(ar.InfoHash[:]))
	q.Set("peer_id", string(ar.PeerId[:]))
	q.Set("port", fmt.Sprintf("%d", ar.Port))
	q.Set("uploaded", strconv.FormatInt(ar.Uploaded, 10))
	q.Set("downloaded", strconv.FormatInt(ar.Downloaded, 10))
	q.Set("left", strconv.FormatUint(ar.Left, 10))
	if ar.Event != None {
		q.Set("event", ar.Event.String())
	}
	// http://stackoverflow.com/questions/17418004/why-does-tracker-server-not-understand-my-request-bittorrent-protocol
	q.Set("compact", "1")
	// According to https://wiki.vuze.com/w/Message_Stream_Encryption.
	q.Set("supportcrypto", "1")

	_url.RawQuery = q.Encode()
}

func announceHTTP(ar *AnnounceRequest, _url *url.URL, host string) (ret AnnounceResponse, err error) {
	_url = httptoo.CopyURL(_url)
	setAnnounceParams(_url, ar)
	req, err := http.NewRequest("GET", _url.String(), nil)
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	io.Copy(&buf, resp.Body)
	if resp.StatusCode != 200 {
		err = fmt.Errorf("response from tracker: %s: %s", resp.Status, buf.String())
		return
	}
	var trackerResponse httpResponse
	err = bencode.Unmarshal(buf.Bytes(), &trackerResponse)
	if err != nil {
		err = fmt.Errorf("error decoding %q: %s", buf.Bytes(), err)
		return
	}
	if trackerResponse.FailureReason != "" {
		err = errors.New(trackerResponse.FailureReason)
		return
	}
	ret.Interval = trackerResponse.Interval
	ret.Leechers = trackerResponse.Incomplete
	ret.Seeders = trackerResponse.Complete
	ret.Peers, err = trackerResponse.UnmarshalPeers()
	return
}

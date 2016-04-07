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

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/util"
)

func init() {
	registerClientScheme("http", newHTTPClient)
}

type httpClient struct {
	url url.URL
}

func (httpClient) Close() error { return nil }

func newHTTPClient(url *url.URL) client {
	return &httpClient{
		url: *url,
	}
}

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

func (me *httpClient) Announce(ar *AnnounceRequest) (ret AnnounceResponse, err error) {
	// retain query parameters from announce URL
	q := me.url.Query()

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
	var reqURL url.URL = me.url
	reqURL.RawQuery = q.Encode()
	resp, err := http.Get(reqURL.String())
	if err != nil {
		return
	}
	defer resp.Body.Close()
	buf := bytes.Buffer{}
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

func (me *httpClient) Connect() error {
	// HTTP trackers do not require a connecting handshake.
	return nil
}

func (me *httpClient) String() string {
	return me.URL()
}

func (me *httpClient) URL() string {
	return me.url.String()
}

package server

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/jpillora/scraper/scraper"
)

var v1SearchHash = regexp.MustCompile(`^(?:[a-fA-F0-9]{40}|[A-Z2-7a-z]{32})$`)

func normalizeSearchResult(row scraper.Result, base *url.URL) error {
	// Tracker extraction is legacy provider data, never an instruction to inject
	// trackers into constructed magnets.
	delete(row, "tracker")
	for _, field := range []string{"url", "torrent", "path"} {
		raw := strings.TrimSpace(row[field])
		if raw == "" {
			delete(row, field)
			continue
		}
		reference, err := url.Parse(raw)
		if err != nil {
			return errors.New("invalid result URL")
		}
		resolved := base.ResolveReference(reference)
		if !safeSearchURL(resolved) {
			return errors.New("unsafe result URL")
		}
		if field == "path" {
			if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
				return errors.New("item path must stay on provider origin")
			}
			row[field] = resolved.RequestURI()
		} else {
			row[field] = resolved.String()
			if field == "url" && row["path"] == "" && resolved.Scheme == base.Scheme && resolved.Host == base.Host {
				row["path"] = resolved.RequestURI()
			}
		}
	}
	if hash := row["infohash"]; hash != "" && !v1SearchHash.MatchString(hash) {
		return errors.New("invalid result hash")
	}
	if magnet := row["magnet"]; magnet != "" {
		u, err := url.Parse(magnet)
		if err != nil || u.Scheme != "magnet" {
			return errors.New("invalid result magnet")
		}
		spec, err := torrent.TorrentSpecFromMagnetUri(magnet)
		if err != nil {
			return errors.New("invalid result magnet")
		}
		if spec.InfoHash == ([20]byte{}) && !spec.InfoHashV2.Ok {
			return errors.New("missing magnet identity")
		}
		// Reject non-HTTP metadata/webseed sources and malformed trackers early;
		// the engine still applies its full destination policy on admission.
		for _, raw := range append(spec.Sources, spec.Webseeds...) {
			u, err := url.Parse(raw)
			if err != nil || !safeSearchURL(u) {
				return errors.New("unsafe magnet source")
			}
		}
		for _, tier := range spec.Trackers {
			for _, raw := range tier {
				u, err := url.Parse(raw)
				if err != nil || u.User != nil || u.Hostname() == "" {
					return errors.New("invalid magnet tracker")
				}
				switch u.Scheme {
				case "http", "https", "udp", "ws", "wss":
				default:
					return errors.New("unsafe magnet tracker")
				}
			}
		}
	}
	return nil
}

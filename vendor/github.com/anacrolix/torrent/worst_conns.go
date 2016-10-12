package torrent

func worseConn(l, r *connection) bool {
	if l.useful() != r.useful() {
		return r.useful()
	}
	if !l.lastHelpful().Equal(r.lastHelpful()) {
		return l.lastHelpful().Before(r.lastHelpful())
	}
	return l.completedHandshake.Before(r.completedHandshake)
}

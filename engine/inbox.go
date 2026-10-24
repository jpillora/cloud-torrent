package engine

import (
	"encoding/hex"
	"errors"
	"github.com/anacrolix/torrent"
)

const maxInboxReceipts = 4096

var ErrAdmissionFull = errors.New("torrent admission limit reached")
var ErrInboxReceiptsFull = errors.New("inbox receipt limit reached; use the manual import API")

// ImportTorrent atomically records the source digest with torrent acceptance.
// Keeping receipts after removal prevents a retained inbox file resurrecting it.
func (e *Engine) ImportTorrent(spec *torrent.TorrentSpec, receipt string) error {
	decoded, err := hex.DecodeString(receipt)
	if err != nil || len(decoded) != 32 {
		return errors.New("invalid inbox receipt")
	}
	return e.addTorrent(spec, hex.EncodeToString(decoded))
}
func (e *Engine) HasInboxReceipt(receipt string) bool {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.inboxReceipts[receipt]
}
func (e *Engine) InboxCounts() (int, int) {
	e.mut.Lock()
	defer e.mut.Unlock()
	return len(e.ts), len(e.inboxReceipts)
}

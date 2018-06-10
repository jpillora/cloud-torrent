package torrent

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/mse"
	pp "github.com/anacrolix/torrent/peer_protocol"
)

type ExtensionBit uint

const (
	ExtensionBitDHT      = 0  // http://www.bittorrent.org/beps/bep_0005.html
	ExtensionBitExtended = 20 // http://www.bittorrent.org/beps/bep_0010.html
	ExtensionBitFast     = 2  // http://www.bittorrent.org/beps/bep_0006.html
)

func handshakeWriter(w io.Writer, bb <-chan []byte, done chan<- error) {
	var err error
	for b := range bb {
		_, err = w.Write(b)
		if err != nil {
			break
		}
	}
	done <- err
}

type (
	peerExtensionBytes [8]byte
)

func (me peerExtensionBytes) String() string {
	return hex.EncodeToString(me[:])
}

func newPeerExtensionBytes(bits ...ExtensionBit) (ret peerExtensionBytes) {
	for _, b := range bits {
		ret.SetBit(b)
	}
	return
}

func (pex peerExtensionBytes) SupportsExtended() bool {
	return pex.GetBit(ExtensionBitExtended)
}

func (pex peerExtensionBytes) SupportsDHT() bool {
	return pex.GetBit(ExtensionBitDHT)
}

func (pex peerExtensionBytes) SupportsFast() bool {
	return pex.GetBit(ExtensionBitFast)
}

func (pex *peerExtensionBytes) SetBit(bit ExtensionBit) {
	pex[7-bit/8] |= 1 << (bit % 8)
}

func (pex peerExtensionBytes) GetBit(bit ExtensionBit) bool {
	return pex[7-bit/8]&(1<<(bit%8)) != 0
}

type handshakeResult struct {
	peerExtensionBytes
	PeerID
	metainfo.Hash
}

// ih is nil if we expect the peer to declare the InfoHash, such as when the
// peer initiated the connection. Returns ok if the handshake was successful,
// and err if there was an unexpected condition other than the peer simply
// abandoning the handshake.
func handshake(sock io.ReadWriter, ih *metainfo.Hash, peerID [20]byte, extensions peerExtensionBytes) (res handshakeResult, ok bool, err error) {
	// Bytes to be sent to the peer. Should never block the sender.
	postCh := make(chan []byte, 4)
	// A single error value sent when the writer completes.
	writeDone := make(chan error, 1)
	// Performs writes to the socket and ensures posts don't block.
	go handshakeWriter(sock, postCh, writeDone)

	defer func() {
		close(postCh) // Done writing.
		if !ok {
			return
		}
		if err != nil {
			panic(err)
		}
		// Wait until writes complete before returning from handshake.
		err = <-writeDone
		if err != nil {
			err = fmt.Errorf("error writing: %s", err)
		}
	}()

	post := func(bb []byte) {
		select {
		case postCh <- bb:
		default:
			panic("mustn't block while posting")
		}
	}

	post([]byte(pp.Protocol))
	post(extensions[:])
	if ih != nil { // We already know what we want.
		post(ih[:])
		post(peerID[:])
	}
	var b [68]byte
	_, err = io.ReadFull(sock, b[:68])
	if err != nil {
		err = nil
		return
	}
	if string(b[:20]) != pp.Protocol {
		return
	}
	missinggo.CopyExact(&res.peerExtensionBytes, b[20:28])
	missinggo.CopyExact(&res.Hash, b[28:48])
	missinggo.CopyExact(&res.PeerID, b[48:68])
	peerExtensions.Add(res.peerExtensionBytes.String(), 1)

	// TODO: Maybe we can just drop peers here if we're not interested. This
	// could prevent them trying to reconnect, falsely believing there was
	// just a problem.
	if ih == nil { // We were waiting for the peer to tell us what they wanted.
		post(res.Hash[:])
		post(peerID[:])
	}

	ok = true
	return
}

// Wraps a raw connection and provides the interface we want for using the
// connection in the message loop.
type deadlineReader struct {
	nc net.Conn
	r  io.Reader
}

func (r deadlineReader) Read(b []byte) (int, error) {
	// Keep-alives should be received every 2 mins. Give a bit of gracetime.
	err := r.nc.SetReadDeadline(time.Now().Add(150 * time.Second))
	if err != nil {
		return 0, fmt.Errorf("error setting read deadline: %s", err)
	}
	return r.r.Read(b)
}

func handleEncryption(
	rw io.ReadWriter,
	skeys mse.SecretKeyIter,
	policy EncryptionPolicy,
) (
	ret io.ReadWriter,
	headerEncrypted bool,
	cryptoMethod mse.CryptoMethod,
	err error,
) {
	if !policy.ForceEncryption {
		var protocol [len(pp.Protocol)]byte
		_, err = io.ReadFull(rw, protocol[:])
		if err != nil {
			return
		}
		rw = struct {
			io.Reader
			io.Writer
		}{
			io.MultiReader(bytes.NewReader(protocol[:]), rw),
			rw,
		}
		if string(protocol[:]) == pp.Protocol {
			ret = rw
			return
		}
	}
	headerEncrypted = true
	ret, cryptoMethod, err = mse.ReceiveHandshake(rw, skeys, func(provides mse.CryptoMethod) mse.CryptoMethod {
		switch {
		case policy.ForceEncryption:
			return mse.CryptoMethodRC4
		case policy.DisableEncryption:
			return mse.CryptoMethodPlaintext
		case policy.PreferNoEncryption && provides&mse.CryptoMethodPlaintext != 0:
			return mse.CryptoMethodPlaintext
		default:
			return mse.DefaultCryptoSelector(provides)
		}
	})
	return
}

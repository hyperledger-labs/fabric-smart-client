/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"
	"encoding/binary"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
)

// preambleTimeout bounds the preamble exchange when a viewProtocolV2 stream opens, so a peer
// that never sends its frame cannot hold the stream's goroutine.
const preambleTimeout = 30 * time.Second

// exchangePreamble writes this node's preamble frame on s and reads the peer's, then accepts it
// with preamble.Accept. Both sides write before they read, so neither waits for the other: the
// stream is shared by all sessions to the peer and either side may start one on it. libp2p has
// authenticated the peer before the stream opens. The exchange ends at preambleTimeout or at
// ctx's deadline, whichever comes first, and resets s when ctx is cancelled. refused reports
// that the peer's frame arrived and preamble.Accept refused it.
func exchangePreamble(ctx context.Context, s network.Stream) (p *preamble.Preamble, refused bool, err error) {
	deadline := time.Now().Add(preambleTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := s.SetDeadline(deadline); err != nil {
		return nil, false, errors.Wrap(err, "failed setting preamble deadline")
	}
	defer func() { _ = s.SetDeadline(time.Time{}) }()
	stop := context.AfterFunc(ctx, func() { _ = s.Reset() })

	if err := writePreambleFrame(s, preamble.LocalBytes()); err != nil {
		stop()
		return nil, false, errors.Wrap(err, "failed writing preamble")
	}
	raw, err := readPreambleFrame(s)
	if !stop() {
		// ctx ended and s is being reset, even if the frame arrived.
		return nil, false, errors.Wrap(ctx.Err(), "preamble exchange cancelled")
	}
	if errors.Is(err, errFrameTooLarge) {
		return nil, true, errors.WithMessage(err, "version")
	}
	if err != nil {
		return nil, false, errors.Wrap(err, "failed reading preamble")
	}
	p, err = preamble.Accept(raw)
	return p, err != nil, err
}

// errFrameTooLarge is returned by readPreambleFrame for a length over preamble.MaxSize.
var errFrameTooLarge = errors.New("preamble frame too large")

// writePreambleFrame writes raw prefixed by its length as a uvarint.
func writePreambleFrame(w io.Writer, raw []byte) error {
	_, err := w.Write(append(binary.AppendUvarint(nil, uint64(len(raw))), raw...))
	return err
}

// readPreambleFrame reads one frame written by writePreambleFrame. It reads the length one
// byte at a time, without a buffered reader, so the view packets that follow the frame stay on
// r, and refuses a length over preamble.MaxSize before allocating for it.
func readPreambleFrame(r io.Reader) ([]byte, error) {
	size, err := binary.ReadUvarint(byteReader{r})
	if err != nil {
		return nil, err
	}
	if size > preamble.MaxSize {
		return nil, errors.Wrapf(errFrameTooLarge, "%d bytes exceeds %d", size, preamble.MaxSize)
	}
	raw := make([]byte, size)
	_, err = io.ReadFull(r, raw)
	return raw, err
}

// byteReader reads single bytes from an io.Reader without buffering ahead.
type byteReader struct{ io.Reader }

func (b byteReader) ReadByte() (byte, error) {
	var buf [1]byte
	_, err := io.ReadFull(b.Reader, buf[:])
	return buf[0], err
}

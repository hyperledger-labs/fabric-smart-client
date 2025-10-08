/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"encoding/binary"
	"math"
	"strconv"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

const maxDelimitedPayloadSize = 10 * 1024 * 1024

// delimitedReader reads the output of a protobuf DelimitedWriter.
// The difference with the protobuf DelimitedReader is that:
// a) We read byte arrays instead of proto.Messages
// b) We don't pass a reader in the constructor
type delimitedReader struct {
	buf            []byte
	expectedLength int
	currentLength  int
	headerBuf      []byte
}

func newDelimitedReader() *delimitedReader {
	return &delimitedReader{
		buf:       []byte{},
		headerBuf: []byte{},
	}
}

func (r *delimitedReader) Read(p []byte) (int, error) {
	if r.expectedLength == 0 {
		r.headerBuf = append(r.headerBuf, p...)
		length, n := binary.Uvarint(r.headerBuf)
		if n <= 0 {
			if n < 0 {
				return 0, errors.New("varint overflow")
			}
			// n == 0 means not enough bytes yet
			return len(p), nil
		}

		// We have a full varint
		if length > maxDelimitedPayloadSize {
			return 0, errors.Errorf("message payload too large [%d], max [%d]", length, maxDelimitedPayloadSize)
		}
		total := length + uint64(n)
		if total > uint64(math.MaxInt) {
			return 0, errors.Errorf("message length overflows int [%d]", total)
		}
		r.expectedLength = int(total)
		if r.expectedLength > len(r.buf) {
			logger.Debugf("Message expected is longer than the buffer. Extending buffer")
			r.buf = make([]byte, r.expectedLength)
		}

		// Move what we read from headerBuf to buf
		copy(r.buf, r.headerBuf)
		r.currentLength = len(r.headerBuf)
		r.headerBuf = r.headerBuf[:0] // Reset for next message

		return len(p), nil
	}

	n := copy(r.buf[r.currentLength:], p)
	r.currentLength += n
	if r.currentLength > r.expectedLength {
		return 0, errors.New("too many elements added [" + strconv.Itoa(len(r.buf)) + "/" + strconv.Itoa(r.expectedLength) + "]")
	}
	return n, nil
}

func (r *delimitedReader) Flush() []byte {
	if r.currentLength < r.expectedLength {
		logger.Debugf("Message not ready yet (%d/%d received): [%s]", r.currentLength, r.expectedLength, string(r.buf[:r.currentLength]))
		return nil
	}
	// The result must be read/copied before we start reading again.
	// Otherwise, we need to byte.Clone the result to avoid overwriting the buffer data.
	// res := buf.Clone(r.buf[:r.currentLength])
	res := r.buf[:r.currentLength]
	r.expectedLength = 0
	r.currentLength = 0
	return res
}

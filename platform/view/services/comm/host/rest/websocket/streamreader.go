/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"encoding/binary"
	"strconv"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// delimitedReader reads the output of a protobuf DelimitedWriter.
// The difference with the protobuf DelimitedReader is that:
// a) We read byte arrays instead of proto.Messages
// b) We don't pass a reader in the constructor
type delimitedReader struct {
	buf            []byte
	expectedLength int
	currentLength  int
}

func newDelimitedReader() *delimitedReader {
	return &delimitedReader{
		buf: []byte{},
	}
}

func (r *delimitedReader) Read(p []byte) (int, error) {
	if r.expectedLength == 0 {
		expectedLength, err := readExpectedLength(p)
		if err != nil {
			return 0, err
		}
		r.expectedLength = expectedLength
		if expectedLength > len(r.buf) {
			logger.Debugf("Message expected is longer than the buffer. Extending buffer")
			r.buf = make([]byte, expectedLength)
		}
	}
	n := copy(r.buf[r.currentLength:], p)
	r.currentLength += n
	if r.currentLength > r.expectedLength {
		return 0, errors.New("too many elements added [" + strconv.Itoa(len(r.buf)) + "/" + strconv.Itoa(r.expectedLength) + "]")
	}
	return n, nil
}

func readExpectedLength(p []byte) (int, error) {
	length, n := binary.Uvarint(p)
	if n <= 0 {
		return 0, errors.Errorf("failed reading expected length [%s]", string(p))
	}
	logger.Debugf("Reading only size: %d + %d", length, len(p))
	return int(length) + len(p), nil
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

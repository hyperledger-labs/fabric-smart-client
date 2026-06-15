/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"encoding/binary"
	"math"
	"strconv"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

const maxDelimitedPayloadSize = 10 * 1024 * 1024

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
		if len(r.headerBuf)+len(p) > binary.MaxVarintLen64+maxDelimitedPayloadSize {
			return 0, errors.Errorf("message header or payload too large")
		}
		r.headerBuf = append(r.headerBuf, p...)
		length, n := binary.Uvarint(r.headerBuf)
		if n <= 0 {
			if n < 0 {
				return 0, errors.New("varint overflow")
			}
			return len(p), nil
		}

		if length > maxDelimitedPayloadSize {
			return 0, errors.Errorf("message payload too large [%d], max [%d]", length, maxDelimitedPayloadSize)
		}
		total := length + uint64(n)
		if total > uint64(math.MaxInt) {
			return 0, errors.Errorf("message length overflows int [%d]", total)
		}
		r.expectedLength = int(total)
		if r.expectedLength > len(r.buf) {
			r.buf = make([]byte, r.expectedLength)
		}

		copy(r.buf, r.headerBuf)
		r.currentLength = len(r.headerBuf)
		r.headerBuf = r.headerBuf[:0]

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
	if r.currentLength < r.expectedLength || r.expectedLength == 0 {
		return nil
	}

	res := r.buf[:r.expectedLength]
	r.expectedLength = 0
	r.currentLength = 0
	return res
}

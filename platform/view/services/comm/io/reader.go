/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"bufio"
	"encoding/binary"
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

// dataReader reads delimited messages as byte arrays
// What differentiates the implementations is the way the messages are delimited
type dataReader interface {
	ReadData() ([]byte, error)
	Close() error
}

// readerCloser reads incoming messages of format F
// What differentiates the implementations is the type of the message and how they are serialized
type readerCloser[F any] interface {
	ReadMsg(F) error
	io.Closer
}

func newVarintReader(reader io.Reader, capacity, maxMessageSize int) dataReader {
	var closer io.Closer
	if c, ok := reader.(io.Closer); ok {
		closer = c
	}
	return &varintReader{r: bufio.NewReaderSize(reader, capacity), closer: closer, maxMessageSize: maxMessageSize}
}

// varintReader reads a varint that contains the length of the message to follow (len([]byte)) and then the message
type varintReader struct {
	r              *bufio.Reader
	closer         io.Closer
	maxMessageSize int
}

func (r *varintReader) ReadData() ([]byte, error) {
	l, err := binary.ReadUvarint(r.r)
	if err != nil {
		return nil, err
	}

	// A non-positive limit is a misconfiguration, not "unlimited": treating it as
	// unlimited would let a peer claim any length and have it allocated below.
	if r.maxMessageSize <= 0 {
		return nil, errors.Errorf("max message size [%d] must be positive", r.maxMessageSize)
	}
	if l > uint64(r.maxMessageSize) {
		return nil, errors.Errorf("message length [%d] exceeds max message size [%d]", l, r.maxMessageSize)
	}

	// The claimed length is bounded but still attacker-controlled, so it is not
	// trusted for the allocation: grow the buffer while reading so a peer must
	// actually send the bytes it claims.
	buffer, err := readFullGrowing(r.r, int(l))
	if err != nil {
		return nil, errors.Wrapf(err, "error reading message of length [%d]", l)
	}
	return buffer, nil
}

// initialReadBufferCap bounds the allocation made before any payload byte has
// been read. A message larger than this grows geometrically as data arrives.
const initialReadBufferCap = 64 * 1024

// maxConsecutiveEmptyReads bounds how many (0, nil) reads are tolerated before
// giving up. io.Reader permits them, so without this a peer holding a stream
// open without sending data would spin its per-stream goroutine forever.
const maxConsecutiveEmptyReads = 100

// readFullGrowing reads exactly n bytes, growing the buffer as data arrives
// rather than allocating n up front. It returns an error if the reader ends
// before n bytes have been read.
func readFullGrowing(r io.Reader, n int) ([]byte, error) {
	buffer := make([]byte, 0, min(n, initialReadBufferCap))
	empty := 0
	for len(buffer) < n {
		if len(buffer) == cap(buffer) {
			// Grow geometrically, never past the message length.
			buffer = append(buffer, 0)[:len(buffer)]
		}
		read, err := r.Read(buffer[len(buffer):min(cap(buffer), n)])
		buffer = buffer[:len(buffer)+read]
		if err != nil {
			// Any error is forgiven once the whole message has arrived, matching
			// io.ReadAtLeast (which io.ReadFull wraps): bufio.Reader's large-read
			// fast path returns the final bytes together with the underlying
			// error, so a complete message must not be rejected because of it.
			if len(buffer) == n {
				break
			}
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		}
		if read == 0 {
			if empty++; empty >= maxConsecutiveEmptyReads {
				return nil, io.ErrNoProgress
			}
			continue
		}
		empty = 0
	}
	return buffer, nil
}

func (r *varintReader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

func newProtoReader(r dataReader) readerCloser[proto.Message] {
	return &protoReader{r: r}
}

// protoReader uses proto.Message as data container
type protoReader struct {
	r dataReader
}

func (r *protoReader) ReadMsg(msg proto.Message) error {
	data, err := r.r.ReadData()
	if err != nil {
		return errors.Wrapf(err, "failed reading data")
	}
	if err := proto.Unmarshal(data, msg); err != nil {
		return errors.Wrapf(err, "failed unmarshalling message")
	}
	return nil
}

func (r *protoReader) Close() error {
	return r.r.Close()
}

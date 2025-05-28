/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"bufio"
	"encoding/binary"
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger()

type Writer interface {
	WriteMsg(proto.Message) error
}

type WriteCloser interface {
	Writer
	io.Closer
}

type Reader interface {
	ReadMsg(msg proto.Message) error
}

type ReadCloser interface {
	Reader
	io.Closer
}

type marshaler interface {
	MarshalTo(data []byte) (n int, err error)
}

func getSize(v interface{}) (int, bool) {
	if sz, ok := v.(interface {
		Size() (n int)
	}); ok {
		return sz.Size(), true
	} else if sz, ok := v.(interface {
		ProtoSize() (n int)
	}); ok {
		return sz.ProtoSize(), true
	} else {
		return 0, false
	}
}

func NewDelimitedWriter(w io.Writer) WriteCloser {
	return &varintWriter{w, make([]byte, binary.MaxVarintLen64), nil}
}

type varintWriter struct {
	w      io.Writer
	lenBuf []byte
	buffer []byte
}

func (w *varintWriter) WriteMsg(msg proto.Message) (err error) {
	var data []byte
	if m, ok := msg.(marshaler); ok {
		n, ok := getSize(m)
		if ok {
			if n+binary.MaxVarintLen64 >= len(w.buffer) {
				w.buffer = make([]byte, n+binary.MaxVarintLen64)
			}
			lenOff := binary.PutUvarint(w.buffer, uint64(n))
			_, err = m.MarshalTo(w.buffer[lenOff:])
			if err != nil {
				return err
			}
			_, err = w.w.Write(w.buffer[:lenOff+n])
			return err
		}
	}

	// fallback
	data, err = proto.Marshal(msg)
	if err != nil {
		return err
	}
	length := uint64(len(data))
	n := binary.PutUvarint(w.lenBuf, length)
	_, err = w.w.Write(w.lenBuf[:n])
	if err != nil {
		return err
	}
	_, err = w.w.Write(data)
	return err
}

func (w *varintWriter) Close() error {
	if closer, ok := w.w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func NewDelimitedReader(r io.Reader, maxSize uint64) ReadCloser {
	var closer io.Closer
	if c, ok := r.(io.Closer); ok {
		closer = c
	}
	return &varintReader{r: bufio.NewReader(r), closer: closer, maxSize: maxSize}
}

type varintReader struct {
	r       *bufio.Reader
	buf     []byte
	closer  io.Closer
	maxSize uint64
}

func (r *varintReader) ReadMsg(msg proto.Message) error {
	length64, err := binary.ReadUvarint(r.r)
	if err != nil {
		return err
	}
	length := int(length64)
	if length64 < 0 {
		return io.ErrShortBuffer
	}
	lenBuf := uint64(len(r.buf))
	if lenBuf < length64 {
		r.buf = make([]byte, length)
	}
	if lenBuf >= r.maxSize {
		logger.Warnf("reading message length [%d]", length64)
	}
	buf := r.buf[:length]
	n, err := io.ReadFull(r.r, buf)
	if err != nil {
		return errors.Wrapf(err, "error reading message of length [%d]", length)
	}
	if n != length {
		return errors.Errorf("failed to read [%d] bytes", length)
	}
	if err := proto.Unmarshal(buf, msg); err != nil {
		return errors.Wrapf(err, "error unmarshalling message of length [%d]", length)
	}
	return nil
}

func (r *varintReader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"encoding/binary"
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

// dataWriter writes delimited messages as byte arrays
// What differentiates the implementations is the way the messages are delimited
type dataWriter interface {
	WriteData([]byte) error
	Close() error
}

// writerCloser writes protobuf messages of format F to the output
// What differentiates the implementations is the type of the message and how they are serialized
type writerCloser[F any] interface {
	WriteMsg(F) error
	io.Closer
}

func newVarintWriter(writer io.Writer) dataWriter {
	var closer io.Closer
	if c, ok := writer.(io.Closer); ok {
		closer = c
	}
	return &varintWriter{w: writer, closer: closer}
}

// varintWriter writes a varint that contains the length of the message to follow (len([]byte)) and then the message
type varintWriter struct {
	w      io.Writer
	closer io.Closer
}

func (w *varintWriter) WriteData(data []byte) error {
	var lenBuf [binary.MaxVarintLen64]byte // lenBuf can be re-used to avoid instantiation
	n := binary.PutUvarint(lenBuf[:], uint64(len(data)))
	if _, err := w.w.Write(lenBuf[:n]); err != nil {
		return errors.Wrapf(err, "could not write message length [%d]", len(data))
	}
	if _, err := w.w.Write(data); err != nil {
		return errors.Wrapf(err, "could not write data [%s]", string(data))
	}
	return nil
}

func (w *varintWriter) Close() error {
	if w.closer != nil {
		return w.closer.Close()
	}
	return nil
}

func newProtoWriter(w dataWriter) writerCloser[proto.Message] {
	return &protoWriter{w: w}
}

// protoWriter uses proto.Message as data container
type protoWriter struct {
	w dataWriter
}

func (w *protoWriter) WriteMsg(msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal the message: [%s]", msg)
	}
	return w.w.WriteData(data)
}

func (w *protoWriter) Close() error { return w.w.Close() }

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

func newVarintReader(reader io.Reader, capacity int) dataReader {
	var closer io.Closer
	if c, ok := reader.(io.Closer); ok {
		closer = c
	}
	return &varintReader{r: bufio.NewReaderSize(reader, capacity), closer: closer}
}

// varintReader reads a varint that contains the length of the message to follow (len([]byte)) and then the message
type varintReader struct {
	r      *bufio.Reader
	closer io.Closer
}

func (r *varintReader) ReadData() ([]byte, error) {
	l, err := binary.ReadUvarint(r.r)
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, l) // We can re-use the buffer to avoid allocations

	if n, err := io.ReadFull(r.r, buffer); err != nil || n != int(l) {
		return nil, errors.Wrapf(err, "error reading message of length [%d]", l)
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

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

type (
	ProtoWriterCloser = writerCloser[proto.Message]
	ProtoReaderCloser = readerCloser[proto.Message]
)

// NewVarintProtoReader creates a reader that uses:
// - varint delimiting
// - protobuf message serialization
func NewVarintProtoReader(reader io.Reader, capacity int) ProtoReaderCloser {
	return newProtoReader(newVarintReader(reader, capacity))
}

// NewVarintProtoWriter creates a writer that uses:
// - varint delimiting
// - protobuf message serialization
func NewVarintProtoWriter(writer io.Writer) ProtoWriterCloser {
	return newProtoWriter(newVarintWriter(writer))
}

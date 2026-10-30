/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package wsproto frames protobuf messages on a WebSocket: one protojson-encoded text frame
// per message. Both ends of the view service's HTTP binding use it, so they agree on the
// encoding by construction. Read ignores unknown fields, as gRPC does, so peers of different
// versions interoperate when a message gains a field.
package wsproto

import (
	"io"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

// Write sends m as one text frame.
func Write(conn *websocket.Conn, m proto.Message) error {
	raw, err := protojson.Marshal(m)
	if err != nil {
		return errors.Wrapf(err, "failed encoding [%T]", m)
	}
	return conn.WriteMessage(websocket.TextMessage, raw)
}

// Read receives one text frame into m. A binary frame is an error. A normal close by the
// peer is io.EOF, as the end of a gRPC stream is.
func Read(conn *websocket.Conn, m proto.Message) error {
	typ, raw, err := conn.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			return io.EOF
		}
		return err
	}
	if typ != websocket.TextMessage {
		return errors.Errorf("unexpected websocket message type [%d], want text", typ)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, m); err != nil {
		return errors.Wrapf(err, "failed decoding [%T]", m)
	}
	return nil
}

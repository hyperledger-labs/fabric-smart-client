/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("view-sdk.host.rest")

// StreamMeta is the first message sent from the websocket client to transmit metadata information
type StreamMeta struct {
	SessionID   string       `json:"session_id"`
	ContextID   string       `json:"context_id"`
	PeerID      host2.PeerID `json:"peer_id"`
	SpanContext []byte       `json:"span_context"`
}

var schemes = map[bool]string{
	true:  "wss",
	false: "ws",
}

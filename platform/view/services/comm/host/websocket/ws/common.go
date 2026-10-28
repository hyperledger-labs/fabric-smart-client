/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

var logger = logging.MustGetLogger()

// StreamMeta is the first message a websocket client sends on a new stream. The server binds
// PeerID to the identity of the client's TLS certificate and rejects the stream on mismatch.
type StreamMeta struct {
	SessionID string      `json:"session_id"`
	ContextID string      `json:"context_id"`
	PeerID    host.PeerID `json:"peer_id"`
	// SpanContext is the client's span, encoded with tracing.MarshalContext. The server uses it
	// as the remote parent of the stream's span; see remoteSpanContext.
	SpanContext []byte `json:"span_context"`
}

// remoteSpanContext decodes meta.SpanContext. Tracing is best effort: a span context that does
// not decode is replaced by an empty one, so whether a stream is accepted never depends on the
// peer's tracing encoding.
func remoteSpanContext(meta *StreamMeta) trace.SpanContext {
	spanContext, err := tracing.UnmarshalContext(meta.SpanContext)
	if err != nil {
		logger.Debugf("ignoring span context of stream [%s,%s] from [%s]: %v", meta.ContextID, meta.SessionID, meta.PeerID, err)
	}
	return spanContext
}

var schemes = map[bool]string{
	true:  "wss",
	false: "ws",
}

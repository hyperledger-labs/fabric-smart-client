/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"encoding/base64"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
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
	// Preamble is the client's encoded preamble.Preamble. The server reads it only after it
	// has bound PeerID to the client's TLS certificate. A client of a release without
	// preambles leaves it empty; see peerPreamble.
	Preamble []byte `json:"preamble,omitempty"`
}

// preambleHeader carries the server's encoded preamble.Preamble, base64 encoded, on the
// websocket upgrade response, so the client learns the server's preamble once per websocket.
// A server of a release without preambles sends no such header.
const preambleHeader = "Fsc-Preamble"

// noPreambleLogged holds the peer IDs that sent no preamble and have been logged once. Its keys
// are authenticated peer IDs, so it is bounded by the number of peers.
var noPreambleLogged sync.Map

// peerPreamble accepts the preamble raw that peerID sent; see preamble.Accept. An empty raw
// comes from a release without preambles: it is accepted, with a nil preamble, and logged once
// per peer.
func peerPreamble(peerID host.PeerID, raw []byte) (*preamble.Preamble, error) {
	if len(raw) == 0 {
		if _, logged := noPreambleLogged.LoadOrStore(peerID, struct{}{}); !logged {
			logger.Infof("peer [%s] sent no preamble, it runs an FSC release without version ranges", peerID)
		}
		return nil, nil
	}
	p, err := preamble.Accept(raw)
	if err != nil {
		return nil, err
	}
	logger.Debugf("peer [%s] runs FSC [%s] revision [%s]", peerID, p.GetFscVersion(), p.GetVcsRevision())
	return p, nil
}

// localPreambleHeader returns the response header that carries this node's preamble.
func localPreambleHeader() http.Header {
	return http.Header{preambleHeader: []string{base64.StdEncoding.EncodeToString(preamble.LocalBytes())}}
}

// serverPreamble returns the preamble that the server peerID sent in header; see peerPreamble.
func serverPreamble(peerID host.PeerID, header http.Header) (*preamble.Preamble, error) {
	raw, err := preambleFromHeader(header)
	if err != nil {
		return nil, err
	}
	return peerPreamble(peerID, raw)
}

// preambleFromHeader returns the encoded preamble in header, empty if there is none. It refuses
// a header that decodes to more than preamble.MaxSize before decoding it.
func preambleFromHeader(header http.Header) ([]byte, error) {
	encoded := header.Get(preambleHeader)
	// A base64 encoding of MaxSize bytes, so the decoder never allocates more for it.
	if len(encoded) > base64.StdEncoding.EncodedLen(preamble.MaxSize) {
		return nil, errors.Errorf("version: preamble header exceeds %d bytes", preamble.MaxSize)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("version: malformed preamble header")
	}
	return raw, nil
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

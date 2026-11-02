/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"bytes"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Reasons a node gives when it refuses to start a responder for a first message. The
// rejection's payload is "<reason>: <detail>".
const (
	// ReasonBusy: the sender already runs the most responders the node allows per peer.
	ReasonBusy = "busy"
	// ReasonNodeBusy: the node already runs the most responders it allows across all peers.
	ReasonNodeBusy = "node_busy"
	// ReasonUnknownView: the node has no responder for the caller view.
	ReasonUnknownView = "unknown_view"
	// ReasonDraining: the node drains before it stops and starts no new work.
	ReasonDraining = "draining"
	// ReasonUnauthorized: the responder's authorizer refused the sender.
	ReasonUnauthorized = "unauthorized"
	// ReasonAuthorizerError: the responder's authorizer failed or timed out.
	ReasonAuthorizerError = "authorizer_error"
)

var (
	// ErrRejected is wrapped by every error RejectionError returns for a known reason.
	ErrRejected = errors.New("remote rejection")
	// ErrUnknownView is wrapped by the error for an unknown_view rejection.
	ErrUnknownView = errors.New("remote rejection: unknown view")
	// ErrBusy is wrapped by the error for a busy or node_busy rejection. A later attempt
	// may succeed.
	ErrBusy = errors.New("remote rejection: busy")
	// ErrDraining is wrapped by the error for a draining rejection. A later attempt on a
	// fresh connection may reach another replica.
	ErrDraining = errors.New("remote rejection: draining")
	// ErrUnauthorized is wrapped by the error for an unauthorized rejection.
	ErrUnauthorized = errors.New("remote rejection: unauthorized")
)

var reasonErrs = []struct {
	prefix []byte
	err    error
}{
	{[]byte(ReasonUnknownView + ": "), ErrUnknownView},
	{[]byte(ReasonBusy + ": "), ErrBusy},
	{[]byte(ReasonNodeBusy + ": "), ErrBusy},
	{[]byte(ReasonDraining + ": "), ErrDraining},
	{[]byte(ReasonUnauthorized + ": "), ErrUnauthorized},
	{[]byte(ReasonAuthorizerError + ": "), ErrRejected},
}

// remoteError is an error reply from the remote end. Its text is the same for every reply, so
// code that matches it keeps working; reason, if set, is what errors.Is matches.
type remoteError struct {
	payload string
	reason  error
}

func (e *remoteError) Error() string { return "received error from remote [" + e.payload + "]" }

func (e *remoteError) Unwrap() []error {
	if e.reason == nil {
		return nil
	}
	return []error{e.reason, ErrRejected}
}

// RejectionError returns the error for msg, a message with status view.ERROR. Its text is
// "received error from remote [<payload>]". If the payload starts with a known reason, the
// error wraps ErrRejected and, except for authorizer_error, the reason's sentinel, such as
// ErrBusy; otherwise it wraps nothing.
//
// The payload is whatever the remote end sent: a responder can send any text as an error, so
// a typed error tells the caller what the remote end claims, not that a check really ran.
func RejectionError(msg *view.Message) error {
	e := &remoteError{payload: string(msg.Payload)}
	for _, r := range reasonErrs {
		if bytes.HasPrefix(msg.Payload, r.prefix) {
			e.reason = r.err
			break
		}
	}
	return e
}

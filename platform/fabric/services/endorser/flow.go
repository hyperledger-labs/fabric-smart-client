/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type receiveTransactionView struct{}

func (r *receiveTransactionView) Call(viewCtx view.Context) (any, error) {
	raw, err := viewCtx.RunView(&receiveView{})
	if err != nil {
		return nil, errors.Wrap(err, "failed receiving transaction content")
	}

	builder := NewBuilder(viewCtx)
	tx, err := builder.NewTransactionFromBytes(raw.([]byte))
	if err != nil {
		return nil, errors.Wrap(err, "failed reconstructing transaction")
	}
	return tx, nil
}

func NewReceiveTransactionView() *receiveTransactionView {
	return &receiveTransactionView{}
}

// receiveTimeout bounds how long a responder waits for the initiator to send
// its next message, so a silent/unresponsive remote peer cannot park this
// goroutine indefinitely. It matches the default receive timeout used
// elsewhere for session-level reads (see session.defaultReceiveTimeout).
const receiveTimeout = 10 * time.Second

type receiveView struct{}

func (s receiveView) Call(viewCtx view.Context) (any, error) {
	session := viewCtx.Session()

	// Wait to receive a state
	ch := session.Receive()

	timeout := time.NewTimer(receiveTimeout)
	defer timeout.Stop()

	var msg *view.Message
	select {
	case msg = <-ch:
	case <-timeout.C:
		return nil, errors.New("timeout reached")
	}

	if msg.Status == view.ERROR {
		return nil, errors.New(string(msg.Payload))
	}
	return msg.Payload, nil
}

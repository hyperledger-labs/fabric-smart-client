/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type receiveTransactionView struct{}

func (r *receiveTransactionView) Call(viewCtx view.Context) (interface{}, error) {
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

type receiveView struct{}

func (s receiveView) Call(viewCtx view.Context) (interface{}, error) {
	session := viewCtx.Session()

	// Wait to receive a state
	ch := session.Receive()

	// TODO: add timeout
	msg := <-ch

	if msg.Status == view.ERROR {
		return nil, errors.New(string(msg.Payload))
	}
	return msg.Payload, nil
}

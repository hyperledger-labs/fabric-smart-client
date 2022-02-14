/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"time"
)

type IsFinalRequest struct {
	Network string
	Channel string
	TxID    string
}

type IsFinalResponse struct {
	Err error
}

type IsFinalInitiatorView struct {
	request   *IsFinalRequest
	recipient view.Identity
}

func NewIsFinalInitiatorView(network, channel, txID string, recipient view.Identity) *IsFinalInitiatorView {
	return &IsFinalInitiatorView{request: &IsFinalRequest{Network: network, Channel: channel, TxID: txID}, recipient: recipient}
}

func (i *IsFinalInitiatorView) Call(context view.Context) (interface{}, error) {
	session, err := session.NewJSON(context, i, i.recipient)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create session to [%s]", i.recipient)
	}
	if err := session.Send(i.request); err != nil {
		return nil, errors.Wrapf(err, "failed to send request to [%s]", i.recipient)
	}
	response := &IsFinalResponse{}
	if err := session.ReceiveWithTimeout(response, 1*time.Minute); err != nil {
		return nil, errors.Wrapf(err, "failed to receive response from [%s]", i.recipient)
	}
	return nil, response.Err
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type FNSProvider interface {
	FabricNetworkService(network string) (driver.FabricNetworkService, error)
}

type IsFinalRequest struct {
	Network string
	Channel string
	TxID    string
}

type IsFinalResponse struct {
	Err error
}

type IsFinalResponderView struct {
	FNSProvider FNSProvider
}

func NewIsFinalResponderView(FNSProvider FNSProvider) *IsFinalResponderView {
	return &IsFinalResponderView{FNSProvider: FNSProvider}
}

func (i *IsFinalResponderView) Call(context view.Context) (interface{}, error) {
	// receive IsFinalRequest struct
	isFinalRequest := &IsFinalRequest{}
	session := session.JSON(context)
	if err := session.Receive(isFinalRequest); err != nil {
		return nil, errors.Wrapf(err, "failed to receive request")
	}

	// check finality
	var err error
	network, err := i.FNSProvider.FabricNetworkService(isFinalRequest.Network)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get network service for %s", isFinalRequest.Network)
	}
	var ch driver.Channel
	ch, err = network.Channel(isFinalRequest.Channel)
	if err == nil {
		err = ch.IsFinal(isFinalRequest.TxID)
	} else {
		err = errors.Wrapf(err, "channel %s not found", isFinalRequest.Channel)
	}

	// send back answer
	if err := session.Send(&IsFinalResponse{Err: err}); err != nil {
		return nil, errors.Wrapf(err, "failed to send response")
	}
	return nil, nil
}

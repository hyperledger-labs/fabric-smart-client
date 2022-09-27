/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

// Event models a chaincode event
type Event = committer.ChaincodeEvent

// EventCallback is used to parse the received events. It takes in input the generated event and
// return a boolean and an error. If the boolean is true, no more events are passed to this callback function.
// If an error occurred, no more events are passed as well.
type EventCallback func(event *Event) (bool, error)

type info struct {
	chaincodeName string
	network       string
	channel       string
	identitiy     view.Identity
}

type RegisterChaincodeCall struct {
	InvokerIdentity  view.Identity
	Network          string
	Channel          string
	ChaincodePath    string
	ChaincodeName    string
	ChaincodeVersion string
	CallBack         EventCallback
}

type listenToEventsView struct {
	*RegisterChaincodeCall
}

//todo add options
func NewListenToEventsView(chaincode string, callBack EventCallback) *listenToEventsView {
	return &listenToEventsView{
		RegisterChaincodeCall: &RegisterChaincodeCall{
			ChaincodeName: chaincode,
			CallBack:      callBack,
		},
	}
}

func (r *listenToEventsView) Call(context view.Context) (interface{}, error) {
	err := r.RegisterChaincodeEvents(context)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// add view to register chaincode
func (r *listenToEventsView) RegisterChaincodeEvents(context view.Context) error {
	// TODO: endorse and then send to ordering
	chaincode, err := getChaincode(context, &info{
		chaincodeName: r.ChaincodeName,
		network:       r.Network,
		channel:       r.Channel,
		identitiy:     r.InvokerIdentity,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to get chaincode [%s:%s:%s]", r.Network, r.Channel, r.ChaincodeName)
	}
	events, err := chaincode.EventListener.ChaincodeEvents()
	if err != nil {
		return errors.Wrapf(err, "failed to get chaincode event channel [%s:%s:%s]", r.Network, r.Channel, r.ChaincodeName)
	}

	go func() {
		for event := range events {
			stop, err := r.CallBack(event)
			if err != nil {
				logger.Errorf("callback failed [%s:%s:%s]: [%s]", r.Network, r.Channel, r.ChaincodeName, err)
				break
			}
			if stop {
				break
			}
		}

		err := chaincode.EventListener.CloseChaincodeEvents()
		if err != nil {
			logger.Errorf("Failed to close event channel [%s:%s:%s]: [%s]", r.Network, r.Channel, r.ChaincodeName, err)
		}
	}()

	return nil
}

func getChaincode(context view.Context, info *info) (*fabric.Chaincode, error) {
	if len(info.chaincodeName) == 0 {
		return nil, errors.Errorf("no chaincode specified")
	}

	fNetwork := fabric.GetFabricNetworkService(context, info.network)
	if fNetwork == nil {
		return nil, errors.Errorf("fabric network service [%s] not found", info.network)
	}
	channel, err := fNetwork.Channel(info.channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", info.network, info.channel)
	}
	if len(info.identitiy) == 0 {
		info.identitiy = fNetwork.IdentityProvider().DefaultIdentity()
	}
	chaincode := channel.Chaincode(info.chaincodeName)
	if chaincode == nil {
		return nil, errors.Errorf("fabric chaincode %s not found", info.chaincodeName)
	}
	return chaincode, nil
}

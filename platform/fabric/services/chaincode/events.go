/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"

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
	Context          context.Context
	InvokerIdentity  view.Identity
	Network          string
	Channel          string
	ChaincodePath    string
	ChaincodeName    string
	ChaincodeVersion string
	CallBack         EventCallback
}

type ListenToEventsView struct {
	*RegisterChaincodeCall
}

// NewListenToEventsView register a listener for the events generated by the passed chaincode
// and call the passed callback when an event is caught.
func NewListenToEventsView(chaincode string, callBack EventCallback) *ListenToEventsView {
	return &ListenToEventsView{
		RegisterChaincodeCall: &RegisterChaincodeCall{
			ChaincodeName: chaincode,
			CallBack:      callBack,
		},
	}
}

func NewListenToEventsViewWithContext(context context.Context, chaincode string, callBack EventCallback) *ListenToEventsView {
	return &ListenToEventsView{
		RegisterChaincodeCall: &RegisterChaincodeCall{
			ChaincodeName: chaincode,
			CallBack:      callBack,
			Context:       context,
		},
	}
}

func (r *ListenToEventsView) Call(context view.Context) (interface{}, error) {
	err := r.RegisterChaincodeEvents(context)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (r *ListenToEventsView) RegisterChaincodeEvents(viewContext view.Context) error {
	// TODO: endorse and then send to ordering
	chaincode, err := getChaincode(viewContext, &info{
		chaincodeName: r.ChaincodeName,
		network:       r.Network,
		channel:       r.Channel,
		identitiy:     r.InvokerIdentity,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to get chaincode [%s:%s:%s]", r.Network, r.Channel, r.ChaincodeName)
	}
	logger.Debugf("getting chaincode events stream for [%s:%s:%s]", r.Network, r.Channel, r.ChaincodeName)
	events, err := chaincode.EventListener.ChaincodeEvents()
	if err != nil {
		return errors.Wrapf(err, "failed to get chaincode event channel [%s:%s:%s]", r.Network, r.Channel, r.ChaincodeName)
	}

	go func() {
		defer func() {
			if err := chaincode.EventListener.CloseChaincodeEvents(); err != nil {
				logger.Errorf("Failed to close event channel [%s:%s:%s]: [%s]", r.Network, r.Channel, r.ChaincodeName, err)
			}
		}()

		ctx := r.Context
		if ctx == nil {
			ctx = context.Background()
		}
		stop := false
		for {
			select {
			case event := <-events:
				logger.Debugf("got chaincode event for [%s:%s:%s], event name [%s]", r.Network, r.Channel, r.ChaincodeName, event.EventName)
				stop, err = r.CallBack(event)
				if err != nil {
					logger.Errorf("callback failed [%s:%s:%s]: [%s]", r.Network, r.Channel, r.ChaincodeName, err)
				}
			case <-ctx.Done():
				originErr := ctx.Err()
				logger.Debugf("context done with err [%s]", originErr)
				_, err = r.CallBack(&committer.ChaincodeEvent{
					Err: errors.Wrapf(originErr, "context done"),
				})
				if err != nil {
					logger.Errorf("callback failed [%s:%s:%s]: [%s]", r.Network, r.Channel, r.ChaincodeName, err)
				}
				stop = true
			case <-viewContext.Context().Done():
				originErr := viewContext.Context().Err()
				logger.Debugf("view context done with err [%s]", originErr)
				_, err = r.CallBack(&committer.ChaincodeEvent{
					Err: errors.Wrapf(originErr, "view context done"),
				})
				if err != nil {
					logger.Errorf("callback failed [%s:%s:%s]: [%s]", r.Network, r.Channel, r.ChaincodeName, err)
				}
				stop = true
			}
			if stop {
				break
			}
		}
	}()

	return nil
}

func getChaincode(context view.Context, info *info) (*fabric.Chaincode, error) {
	if len(info.chaincodeName) == 0 {
		return nil, errors.Errorf("no chaincode specified")
	}

	fNetwork, err := fabric.GetFabricNetworkService(context, info.network)
	if err != nil {
		return nil, err
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

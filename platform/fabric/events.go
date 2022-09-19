/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	committer "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

type EventListener struct {
	chaincodeListener chan *committer.ChaincodeEvent
	sp                view2.ServiceProvider
	chaincodeName     string
}

func (e *EventListener) ChaincodeEvents() (chan *committer.ChaincodeEvent, error) {
	//todo - check whether channel doesn't exist and is closed
	//todo - channel length should be number of subscribers
	e.chaincodeListener = make(chan *committer.ChaincodeEvent, 1)
	subscriber, err := events.GetSubscriber(e.sp)
	if err != nil {
		return nil, err
	}
	subscriber.Subscribe(e.chaincodeName, e)
	return e.chaincodeListener, nil
}

func newEventListener(sp view2.ServiceProvider, chaincodeName string) *EventListener {
	return &EventListener{
		sp:            sp,
		chaincodeName: chaincodeName,
	}
}

func (e *EventListener) OnReceive(event events.Event) {
	//todo filter events based on options passed - start block last transactionid
	e.chaincodeListener <- event.Message().(*committer.ChaincodeEvent)
}

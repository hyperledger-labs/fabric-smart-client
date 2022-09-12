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

func (e *EventListener) ChaincodeEvents() <-chan *committer.ChaincodeEvent {
	e.chaincodeListener = make(chan *committer.ChaincodeEvent, 1)
	subscriber, _ := events.GetSubscriber(e.sp)
	subscriber.Subscribe(e.chaincodeName, e)
	return e.chaincodeListener
}

func newEventListener(sp view2.ServiceProvider, name string) *EventListener {
	return &EventListener{
		sp:            sp,
		chaincodeName: name,
	}
}

func (e *EventListener) OnReceive(event events.Event) {
	e.chaincodeListener <- event.Message().(*committer.ChaincodeEvent)
}

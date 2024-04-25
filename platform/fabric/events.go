/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

// EventListener models the parameters to use for chaincode listening.
type EventListener struct {
	chaincodeListener chan *committer.ChaincodeEvent
	subscriber        events.Subscriber
	chaincodeName     string
}

func newEventListener(subscriber events.Subscriber, chaincodeName string) *EventListener {
	return &EventListener{
		chaincodeName: chaincodeName,
		subscriber:    subscriber,
	}
}

// ChaincodeEvents returns a channel from which chaincode events emitted by transaction functions in the specified chaincode can be read.
func (e *EventListener) ChaincodeEvents() (chan *committer.ChaincodeEvent, error) {
	e.chaincodeListener = make(chan *committer.ChaincodeEvent, 1)
	e.subscriber.Subscribe(e.chaincodeName, e)
	return e.chaincodeListener, nil
}

// CloseChaincodeEvents closes the channel from which chaincode events are read.
func (e *EventListener) CloseChaincodeEvents() error {
	close(e.chaincodeListener)
	e.subscriber.Unsubscribe(e.chaincodeName, e)

	return nil
}

func (e *EventListener) OnReceive(event events.Event) {
	//todo filter events based on options passed - start block, last transactionid
	e.chaincodeListener <- event.Message().(*committer.ChaincodeEvent)
}

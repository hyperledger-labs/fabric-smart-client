/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

// EventListener models the parameters to use for chaincode listening.
type EventListener struct {
	sync.RWMutex
	chaincodeListener chan *committer.ChaincodeEvent
	subscriber        events.Subscriber
	chaincodeName     string
	closing           bool
}

func newEventListener(subscriber events.Subscriber, chaincodeName string) *EventListener {
	return &EventListener{
		chaincodeName: chaincodeName,
		subscriber:    subscriber,
	}
}

// ChaincodeEvents returns a channel from which chaincode events emitted by transaction functions in the specified chaincode can be read.
func (e *EventListener) ChaincodeEvents() chan *committer.ChaincodeEvent {
	e.chaincodeListener = make(chan *committer.ChaincodeEvent, 1)
	e.subscriber.Subscribe(e.chaincodeName, e)
	return e.chaincodeListener
}

// CloseChaincodeEvents closes the channel from which chaincode events are read.
func (e *EventListener) CloseChaincodeEvents() {
	e.Lock()
	e.closing = true
	e.Unlock()

	e.subscriber.Unsubscribe(e.chaincodeName, e)
	close(e.chaincodeListener)
}

// OnReceive pushes events to the listener
func (e *EventListener) OnReceive(event events.Event) {
	e.RLock()
	defer e.RUnlock()
	if !e.closing {
		e.chaincodeListener <- event.Message().(*committer.ChaincodeEvent)
	}
}

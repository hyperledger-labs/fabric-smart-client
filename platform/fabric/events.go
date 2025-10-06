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
	chaincodeListener chan *committer.ChaincodeEvent
	subscriber        events.Subscriber
	chaincodeName     string

	subscribeOnce sync.Once

	middleCh chan *committer.ChaincodeEvent
	closing  chan struct{}
	closed   chan struct{}
}

func newEventListener(subscriber events.Subscriber, chaincodeName string) *EventListener {
	return &EventListener{
		chaincodeName:     chaincodeName,
		subscriber:        subscriber,
		chaincodeListener: make(chan *committer.ChaincodeEvent),
		middleCh:          make(chan *committer.ChaincodeEvent),
		closing:           make(chan struct{}),
		closed:            make(chan struct{}),
	}
}

// ChaincodeEvents returns a channel from which chaincode events emitted by transaction functions in the specified chaincode can be read.
func (e *EventListener) ChaincodeEvents() <-chan *committer.ChaincodeEvent {
	e.subscribeOnce.Do(func() {

		go func() {
			exit := func(v *committer.ChaincodeEvent, needSend bool) {
				close(e.closed)
				if needSend {
					e.chaincodeListener <- v
				}
				close(e.chaincodeListener)
			}

			for {
				select {
				case <-e.closing:
					exit(nil, false)
					return
				case v := <-e.middleCh:
					select {
					case <-e.closing:
						exit(v, true)
						return
					case e.chaincodeListener <- v:
					}
				}
			}
		}()

		e.subscriber.Subscribe(e.chaincodeName, e)
	})

	return e.chaincodeListener
}

// CloseChaincodeEvents closes the channel from which chaincode events are read.
func (e *EventListener) CloseChaincodeEvents() {
	select {
	case e.closing <- struct{}{}:
		e.subscriber.Unsubscribe(e.chaincodeName, e)
		<-e.closed
	case <-e.closed:
	}
}

// OnReceive pushes events to the listener
func (e *EventListener) OnReceive(event events.Event) {
	if event == nil {
		return
	}

	select {
	case <-e.closed:
		return
	default:
	}

	select {
	case <-e.closed:
		return
	case e.middleCh <- event.Message().(*committer.ChaincodeEvent):
	}
}

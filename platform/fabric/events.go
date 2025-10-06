/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

const (
	defaultBufferLen   = 10
	defaultRecvTimeout = 10 * time.Millisecond
)

// EventListener implements the event.Listener interface and provides
// a best-effort mechanism for receiving chaincode events.
//
// Internally, it maintains a buffered channel of size `bufferLen` for pending events.
// When the buffer is full, new events are retained for up to `recvTimeout` to allow
// consumers a chance to catch up. If the buffer remains full after this timeout,
// the pending event is dropped to avoid blocking the producer.
//
// This implementation prioritizes system responsiveness over guaranteed delivery;
// consumers should tolerate occasional event loss.
type EventListener struct {
	eventCh       chan *committer.ChaincodeEvent // this is our main event channel
	subscriber    events.Subscriber
	chaincodeName string

	subscribeOnce sync.Once
	middleCh      chan *committer.ChaincodeEvent
	closing       chan struct{}
	closed        chan struct{}

	recvTimeout time.Duration
}

// newEventListener create a `EventListener` with `defaultBufferLen` and `defaultRecvTimeout`.
func newEventListener(subscriber events.Subscriber, chaincodeName string) *EventListener {
	return &EventListener{
		chaincodeName: chaincodeName,
		subscriber:    subscriber,
		eventCh:       make(chan *committer.ChaincodeEvent, defaultBufferLen),
		middleCh:      make(chan *committer.ChaincodeEvent),
		closing:       make(chan struct{}),
		closed:        make(chan struct{}),
		recvTimeout:   defaultRecvTimeout,
	}
}

// ChaincodeEvents returns a channel from which chaincode events emitted by transaction functions in the specified chaincode can be read.
func (e *EventListener) ChaincodeEvents() <-chan *committer.ChaincodeEvent {
	e.subscribeOnce.Do(func() {
		// when a consumer first time calls ChaincodeEvents, we set up the event subscription for the chaincode name
		// and start this goroutine for graceful closing
		go func() {
			// our shutdown helper function
			exit := func(v *committer.ChaincodeEvent, needSend bool) {
				close(e.closed)
				e.subscriber.Unsubscribe(e.chaincodeName, e)
				if needSend {
					e.eventCh <- v
				}
				close(e.eventCh)
			}

			for {
				select {
				case <-e.closing:
					exit(nil, false)
					return
				case v := <-e.middleCh:
					// we have a new event v received via OnReceive
					select {
					case <-e.closing:
						exit(v, true)
						return
					case e.eventCh <- v:
						// forward event v to event channel
					}
				}
			}
		}()

		// finally we create our subscription
		e.subscriber.Subscribe(e.chaincodeName, e)
	})

	// we always return the event channel; if it is closed
	return e.eventCh
}

// CloseChaincodeEvents closes the channel from which chaincode events are read.
func (e *EventListener) CloseChaincodeEvents() {
	select {
	case e.closing <- struct{}{}:
		<-e.closed
	case <-e.closed:
	}
}

// OnReceive pushes events to the listener.
// The event is dropped, if it cannot be delivered to the event channel before the recvTimeout is fired.
func (e *EventListener) OnReceive(event events.Event) {
	if event == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.recvTimeout)
	defer cancel()

	// we check if our event listener is already closed
	// we do this extra select to prioritize the close channel (https://go.dev/ref/spec#Select_statements).
	select {
	case <-e.closed:
		return
	default:
	}

	select {
	case <-e.closed:
		return
	case <-ctx.Done():
		// if the event cannot send to the middleCh before the recvTimeout is fired,
		// we return to not further block the event notifier
		return
	case e.middleCh <- event.Message().(*committer.ChaincodeEvent):
	}
}

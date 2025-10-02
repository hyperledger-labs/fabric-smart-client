/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	context2 "context"
	"encoding/json"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type MultipleListenersView struct {
	*MultipleListeners
}

type MultipleListeners struct {
	Function      string
	EventName     string
	ListenerCount int
}

type MultipleListenersReceived struct {
	Events []*chaincode.Event
}

func (c *MultipleListenersView) Call(context view.Context) (interface{}, error) {
	eventReceived := make([]*chaincode.Event, c.ListenerCount)
	cancels := make([]context2.CancelFunc, c.ListenerCount)

	eventCh := make(chan struct{}, c.ListenerCount)
	defer close(eventCh)

	closeCh := make(chan struct{}, c.ListenerCount)
	defer close(closeCh)

	logger.Infof("Register %v event listener", c.ListenerCount)
	var wg sync.WaitGroup
	for i := 0; i < c.ListenerCount; i++ {
		wg.Add(1)
		// we first setup n (ListenerCounter) event listener
		go func(i int) {
			defer wg.Done()
			callBack := func(event *chaincode.Event) (bool, error) {
				// simulate processing, to test concurrency issues
				time.Sleep(rand.N(500 * time.Millisecond))

				if event.Err != nil && strings.Contains(event.Err.Error(), "context done") {
					// the listener received a close event
					closeCh <- struct{}{}
					return false, nil
				}

				// store the event and notify
				eventReceived[i] = event
				eventCh <- struct{}{}
				return false, nil
			}

			ctx, cancelFunc := context2.WithCancel(context.Context())
			cancels[i] = cancelFunc
			_, err := context.RunView(chaincode.NewListenToEventsViewWithContext(ctx, "events", callBack))
			assert.NoError(err, "failed to listen to events")
		}(i)
	}
	// wait until all event listeners are registered
	wg.Wait()

	// Invoke the chaincode to trigger all the listeners once
	_, err := context.RunView(chaincode.NewInvokeView("events", c.Function))
	assert.NoError(err, "Failed Running Invoke View")

	logger.Infof("Waiting for all listeners to complete the event ...")
	for range c.ListenerCount {
		<-eventCh
	}

	// next, we create a steady load of 10 transactions per second
	// after a few transaction invoked we cancel all active listeners

	invokes := 50
	continueAfter := 20

	done := make(chan struct{})
	invCh := make(chan struct{}, invokes)
	defer close(invCh)

	// create a another consumer
	go func() {
		for {
			select {
			case <-eventCh:
			case <-done:
				return
			}
		}
	}()

	logger.Infof("Start transacting ...")
	go func() {
		for i := 0; i < invokes; i++ {
			time.Sleep(100 * time.Millisecond)
			_, err := context.RunView(chaincode.NewInvokeView("events", c.Function))
			assert.NoError(err, "Failed Running Invoke View")
			invCh <- struct{}{}
		}
		// we sent all our transactions
		close(done)
	}()

	logger.Infof("Wait for %v transaction to be invoked", continueAfter)
	// wait for the first events to arrive
	for range continueAfter {
		<-invCh
	}

	logger.Infof("Start canceling ...")
	// cancel subscriptions while new transactions are still coming in
	for _, cancel := range cancels {
		go func(cn context2.CancelFunc) {
			cn()
		}(cancel)
	}

	logger.Infof("Wait for a close events triggered via callbacks")
	// all listener should eventually receive a close event
	for range c.ListenerCount {
		<-closeCh
	}

	logger.Infof("waiting for all our transactions to finish")
	<-done

	logger.Infof("done for today")
	return &MultipleEventsReceived{
		Events: eventReceived,
	}, nil
}

type MultipleListenersViewFactory struct{}

func (c *MultipleListenersViewFactory) NewView(in []byte) (view.View, error) {
	f := &MultipleListenersView{MultipleListeners: &MultipleListeners{}}
	err := json.Unmarshal(in, f)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

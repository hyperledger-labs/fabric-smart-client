/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	context2 "context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type MultipleListenersView struct {
	*MultipleListeners
}

type MultipleListeners struct {
	Function      string
	EventName     string
	ListenerCount uint8
}

type MultipleListenersReceived struct {
	Events []*chaincode.Event
}

func (c *MultipleListenersView) Call(context view.Context) (interface{}, error) {
	wg := sync.WaitGroup{}
	wg.Add(int(c.ListenerCount))

	mtx := sync.Mutex{}
	eventReceived := make([]*chaincode.Event, c.ListenerCount)
	cancels := make([]context2.CancelFunc, c.ListenerCount)

	rec := uint8(0)

	for i := 0; i < int(c.ListenerCount); i++ {
		go func(i int) {
			callBack := func(event *chaincode.Event) (bool, error) {
				// simulate processing, to test concurrency issues
				time.Sleep(500 * time.Millisecond)
				if event.Err != nil && strings.Contains(event.Err.Error(), "context done") {
					logger.Infof(">>>> close %d", i)
					return false, nil
				}

				logger.Debugf("Chaincode Event Received in callback %s", event.EventName)
				mtx.Lock()
				eventReceived[i] = event
				// don't call done too often
				if rec < c.ListenerCount {
					wg.Done()
				}
				rec++
				mtx.Unlock()

				return false, nil
			}

			ctx, cancelFunc := context2.WithCancel(context.Context())
			cancels[i] = cancelFunc
			_, err := context.RunView(chaincode.NewListenToEventsViewWithContext(ctx, "events", callBack))
			assert.NoError(err, "failed to listen to events")
		}(i)
	}

	// Invoke the chaincode to trigger all the listeners once
	_, err := context.RunView(chaincode.NewInvokeView("events", c.Function))
	assert.NoError(err, "Failed Running Invoke View")
	wg.Wait()

	invokes := 50
	continueAfter := 20

	// create a steady load of 10 transactions per second
	wg.Add(continueAfter)
	for i := 0; i < invokes; i++ {
		go func(j int) {
			time.Sleep(100 * time.Millisecond)
			_, err := context.RunView(
				chaincode.NewInvokeView(
					"events",
					c.Function,
				),
			)
			assert.NoError(err, "Failed Running Invoke View")
			if j < continueAfter {
				wg.Done()
			}
		}(i)
	}

	// wait for the first events to arrive
	wg.Wait()

	// cancel subscriptions while new transactions are still coming in
	wg.Add(int(c.ListenerCount))
	for _, cancel := range cancels {
		go func(cn context2.CancelFunc) {
			cn()
			wg.Done()
		}(cancel)
	}
	_, err = context.RunView(chaincode.NewInvokeView("events", c.Function))
	assert.NoError(err, "Failed Running Invoke View")

	wg.Wait()

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

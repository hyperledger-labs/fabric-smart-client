/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type MultipleEventsView struct {
	*MultipleEvents
}

type MultipleEvents struct {
	Functions  []string
	EventCount uint8
}

type MultipleEventsReceived struct {
	Events []*chaincode.Event
}

func (c *MultipleEventsView) Call(context view.Context) (interface{}, error) {
	wg := sync.WaitGroup{}
	wg.Add(int(c.EventCount))
	var eventReceived []*chaincode.Event

	// Register for events
	callBack := func(event *chaincode.Event) (bool, error) {
		logger.Debugf("Chaincode Event Received in callback %s", event.EventName)
		eventReceived = append(eventReceived, event)
		defer wg.Done()
		if len(eventReceived) != int(c.EventCount) {
			return false, nil
		}
		return true, nil
	}

	_, err := context.RunView(chaincode.NewListenToEventsView("events", callBack))
	assert.NoError(err, "failed to listen to events")

	for _, function := range c.Functions {
		// Invoke the chaincode
		_, err = context.RunView(chaincode.NewInvokeView("events", function))
		assert.NoError(err, "Failed Running Invoke View")
	}

	// wait for the event to arrive
	wg.Wait()

	return &MultipleEventsReceived{
		Events: eventReceived,
	}, nil
}

type MultipleEventsViewFactory struct{}

func (c *MultipleEventsViewFactory) NewView(in []byte) (view.View, error) {
	f := &MultipleEventsView{MultipleEvents: &MultipleEvents{}}
	err := json.Unmarshal(in, f)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

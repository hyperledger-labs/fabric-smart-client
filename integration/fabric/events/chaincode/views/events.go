/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"go.uber.org/zap/zapcore"
)

type EventsView struct {
	*Events
}
type Events struct {
	Functions  []string
	EventCount uint8
	EventName  string
}
type EventReceived struct {
	Event *chaincode.Event
}

var logger = flogging.MustGetLogger("view-events")

func (c *EventsView) Call(context view.Context) (interface{}, error) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	var eventReceived *chaincode.Event

	// Register for events
	callBack := func(event *chaincode.Event) (bool, error) {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("Chaincode Event Received in callback %s", event.EventName)
		}
		if event.EventName == c.EventName {
			eventReceived = event
			defer wg.Done()
			return true, nil
		}
		return false, nil
	}
	for _, function := range c.Functions {
		_, err := context.RunView(chaincode.NewListenToEventsView("asset_transfer_events", callBack))
		assert.NoError(err, "failed to listen to events")
		// Invoke the chaincode
		_, err = context.RunView(
			chaincode.NewInvokeView(
				"asset_transfer_events",
				function,
			),
		)
		assert.NoError(err, "Failed Running Invoke View ")
	}

	// wait for the event to arriver
	wg.Wait()
	return &EventReceived{
		Event: eventReceived,
	}, nil
}

func (c *EventsView) NewView(in []byte) (view.View, error) {
	f := &EventsView{Events: &Events{}}
	err := json.Unmarshal(in, f)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

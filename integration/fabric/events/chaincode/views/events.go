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

var logger = flogging.MustGetLogger("view-events")

type EventsView struct {
	*Events
}
type Events struct {
	Function  string
	EventName string
}

type EventReceived struct {
	Event *chaincode.Event
}

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

	_, err := context.RunView(chaincode.NewListenToEventsView("events", callBack))
	assert.NoError(err, "failed to listen to events")
	// Invoke the chaincode
	_, err = context.RunView(
		chaincode.NewInvokeView(
			"events",
			c.Function,
		),
	)
	assert.NoError(err, "Failed Running Invoke View ")

	// wait for the event to arriver
	wg.Wait()
	return &EventReceived{
		Event: eventReceived,
	}, nil
}

type EventsViewFactory struct{}

func (c *EventsViewFactory) NewView(in []byte) (view.View, error) {
	f := &EventsView{Events: &Events{}}
	err := json.Unmarshal(in, f)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}

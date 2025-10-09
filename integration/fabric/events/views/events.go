/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	context2 "context"
	"encoding/json"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

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
	var eventError error

	// Register for events
	callBack := func(event *chaincode.Event) (bool, error) {
		logger.Debugf("Chaincode Event Received in callback %s", event.EventName)
		if event.Err != nil {
			eventError = event.Err
			wg.Done()
			return true, nil
		}

		if event.EventName == c.EventName {
			eventReceived = event
			wg.Done()
			return true, nil
		}

		return false, nil
	}

	// Test timeout
	ctx, cancelFunc := context2.WithTimeout(context.Context(), 10*time.Second)
	defer cancelFunc()
	_, err := context.RunView(chaincode.NewListenToEventsViewWithContext(ctx, "events", callBack))
	assert.NoError(err, "failed to listen to events")
	wg.Wait()
	assert.Error(eventError, "expected error to have happened")
	assert.Equal(errors.Cause(eventError), context2.DeadlineExceeded, "expected deadline exceeded error")
	cancelFunc()

	// Now invoke the chaincode
	// Invoke the chaincode
	wg.Add(1)
	eventReceived = nil
	eventError = nil
	ctx1, cancelFunc1 := context2.WithTimeout(context.Context(), 1*time.Minute)
	defer cancelFunc1()
	_, err = context.RunView(chaincode.NewListenToEventsViewWithContext(ctx1, "events", callBack))
	assert.NoError(err, "failed to listen to events")
	_, err = context.RunView(
		chaincode.NewInvokeView(
			"events",
			c.Function,
		),
	)
	assert.NoError(err, "Failed Running Invoke View")
	wg.Wait()
	assert.NoError(eventError, "expected error to not have happened")
	assert.NotNil(eventReceived, "expected to have received an event")

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

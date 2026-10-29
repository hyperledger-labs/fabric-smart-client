/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package simple

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/mock"
)

var (
	_ events.Subscriber = (*EventBus)(nil)
	_ events.Publisher  = (*EventBus)(nil)
)

func TestNewEventBus(t *testing.T) {
	t.Parallel()

	notifier := NewEventBus()
	require.NotNil(t, notifier)
	require.NotNil(t, notifier.handlers)
}

func TestEventBusPublishSubscribe(t *testing.T) {
	t.Parallel()

	// setup returns an empty bus and a listener not yet subscribed to it.
	setup := func(t *testing.T) (*EventBus, *mock.Listener) {
		t.Helper()
		notifier := NewEventBus()
		require.Empty(t, notifier.handlers)
		return notifier, &mock.Listener{}
	}

	newEvent := func() *mock.Event {
		event := &mock.Event{}
		event.TopicReturns("topicAAA")
		event.MessageReturns("HelloWorld")
		return event
	}

	t.Run("Subscribe", func(t *testing.T) {
		t.Parallel()
		notifier, listener := setup(t)

		notifier.Subscribe("topicAAA", listener)
		require.NotEmpty(t, notifier.handlers)

		notifier.Unsubscribe("topicAAA", nil)
		require.NotEmpty(t, notifier.handlers)

		notifier.Unsubscribe("topicBBB", listener)
		require.NotEmpty(t, notifier.handlers)

		notifier.Unsubscribe("topicAAA", listener)
		require.Empty(t, notifier.handlers)

		notifier.Unsubscribe("topicAAA", nil)
		require.Empty(t, notifier.handlers)

		notifier.Subscribe("topicAAA", listener)
		require.NotEmpty(t, notifier.handlers)

		notifier.Unsubscribe("topicAAA", &mock.Listener{})
		require.NotEmpty(t, notifier.handlers)
	})

	t.Run("Publish", func(t *testing.T) {
		t.Parallel()
		notifier, listener := setup(t)
		event := newEvent()

		notifier.Publish(nil)
		require.Equal(t, 0, listener.OnReceiveCallCount())

		notifier.Publish(event)
		require.Equal(t, 1, event.TopicCallCount())
		require.Equal(t, 0, listener.OnReceiveCallCount())

		notifier.Subscribe("topicAAA", listener)
		require.NotEmpty(t, notifier.handlers)

		for i := range 100000 {
			notifier.Publish(event)
			require.Equal(t, i+2, event.TopicCallCount())
			require.Equal(t, i+1, listener.OnReceiveCallCount())
		}
	})

	t.Run("many events", func(t *testing.T) {
		t.Parallel()
		notifier, listener := setup(t)
		event := newEvent()

		notifier.Subscribe("topicAAA", listener)
		require.NotEmpty(t, notifier.handlers)

		for i := range 100000 {
			notifier.Publish(event)
			require.Equal(t, i+1, event.TopicCallCount())
			require.Equal(t, i+1, listener.OnReceiveCallCount())
		}
	})

	t.Run("many topics", func(t *testing.T) {
		t.Parallel()
		notifier, _ := setup(t)

		for i := range 10000 {
			notifier.Subscribe(fmt.Sprintf("topic_%d", i), &mock.Listener{})
			require.Len(t, notifier.handlers, i+1)
		}
	})

	t.Run("many listeners", func(t *testing.T) {
		t.Parallel()
		notifier, _ := setup(t)

		for i := range 10000 {
			notifier.Subscribe("topic", &mock.Listener{})
			require.Len(t, notifier.handlers, 1)
			require.Len(t, notifier.handlers["topic"], i+1)
		}
	})
}

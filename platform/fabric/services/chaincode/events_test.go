/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
)

func TestEventsView(t *testing.T) {
	t.Parallel()

	mockCallback := func(event *chaincode.Event) (bool, error) {
		return true, nil
	}

	t.Run("NewListenToEventsView", func(t *testing.T) {
		t.Parallel()
		v := chaincode.NewListenToEventsView("my-chaincode", mockCallback)
		require.NotNil(t, v)
		require.Equal(t, "my-chaincode", v.ChaincodeName)
		require.NotNil(t, v.CallBack)
	})

	t.Run("NewListenToEventsViewWithContext", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		v := chaincode.NewListenToEventsViewWithContext(ctx, "my-chaincode", mockCallback)
		require.NotNil(t, v)
		require.Equal(t, "my-chaincode", v.ChaincodeName)
		require.NotNil(t, v.CallBack)
		require.Equal(t, ctx, v.Ctx)
	})

	t.Run("Events errors", func(t *testing.T) {
		t.Parallel()

		v := chaincode.NewListenToEventsView("", mockCallback)
		_, err := v.Call(nil)
		require.ErrorContains(t, err, "no chaincode specified")

		// view.Context GetService network error
		mockCtx := &mock.Context{}
		mockCtx.GetServiceReturns(nil, errors.New("network error"))

		v2 := chaincode.NewListenToEventsView("my-chaincode", mockCallback)
		_, err = v2.Call(mockCtx)
		require.ErrorContains(t, err, "network error")
	})

	t.Run("Events happy path", func(t *testing.T) {
		t.Parallel()

		sub := &mockSubscriber{}
		mockCtx, _, _ := setupMockContextWithSubscriber(t, sub)

		eventCh := make(chan *chaincode.Event, 1)
		mockCallback := func(event *chaincode.Event) (bool, error) {
			eventCh <- event
			return true, nil
		}

		v := chaincode.NewListenToEventsView("my-chaincode", mockCallback)
		_, err := v.Call(mockCtx)
		require.NoError(t, err)

		ccEvent := &committer.ChaincodeEvent{EventName: "test-event"}

		require.Eventually(t, func() bool {
			return sub.listener != nil
		}, 2*time.Second, 10*time.Millisecond)

		sub.listener.OnReceive(&mockEvent{topic: "my-chaincode", msg: ccEvent})

		select {
		case ev := <-eventCh:
			require.Equal(t, "test-event", ev.EventName)
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for event")
		}
	})
}

type mockSubscriber struct {
	listener events.Listener
}

func (m *mockSubscriber) Subscribe(topic string, listener events.Listener) {
	m.listener = listener
}

func (m *mockSubscriber) Unsubscribe(topic string, listener events.Listener) {}

type mockEvent struct {
	topic string
	msg   any
}

func (m *mockEvent) Topic() string { return m.topic }
func (m *mockEvent) Message() any  { return m.msg }

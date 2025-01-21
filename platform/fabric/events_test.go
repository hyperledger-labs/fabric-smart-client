/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/stretchr/testify/assert"
)

type mockSubscriber struct {
	listener events.Listener
	m        sync.RWMutex
}

func (m *mockSubscriber) Subscribe(chaincodeName string, listener events.Listener) {
	m.m.Lock()
	defer m.m.Unlock()
	m.listener = listener
}

func (m *mockSubscriber) Unsubscribe(chaincodeName string, listener events.Listener) {
	m.m.Lock()
	defer m.m.Unlock()
	m.listener = nil
}

func (m *mockSubscriber) Publish(chaincodeName string, event *committer.ChaincodeEvent) {
	m.m.RLock()
	defer m.m.RUnlock()
	if m.listener != nil {
		m.listener.OnReceive(event)
	}
}

func TestEventListener(t *testing.T) {
	subscriber := &mockSubscriber{}
	listener := newEventListener(subscriber, "testChaincode")
	eventChannel := listener.ChaincodeEvents()

	var wg sync.WaitGroup

	// Publish events
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			subscriber.Publish("testChaincode", &committer.ChaincodeEvent{Payload: []byte(fmt.Sprintf("event %d", i))})
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Add(1)
	// Stop while the publisher is still publishing
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	go func() {
		defer wg.Done()
		for {
			select {
			case event := <-eventChannel:
				assert.NotNil(t, event)
			case <-ctx.Done():
				listener.CloseChaincodeEvents()
				return
			}
		}
	}()
	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// Ensure the channel is closed
	select {
	case _, ok := <-eventChannel:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	default:
		t.Fatal("expected to read from closed channel")
	}
}

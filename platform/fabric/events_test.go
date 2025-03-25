/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/stretchr/testify/assert"
)

const (
	publicationSnooze = time.Millisecond
	timeout           = 75 * time.Millisecond
	waitFor           = 4 * timeout
	tick              = timeout / 10
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
	ch := listener.ChaincodeEvents()

	done := make(chan bool)

	var wg sync.WaitGroup
	wg.Add(2)

	// Publish events
	go func() {
		defer wg.Done()
		msg := []byte("some msg")
		for {
			select {
			case <-done:
				return
			default:
				subscriber.Publish("testChaincode", &committer.ChaincodeEvent{Payload: msg})
				time.Sleep(publicationSnooze)
			}
		}
	}()

	// Stop while the publisher is still publishing
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		defer wg.Done()
		for {
			select {
			case event := <-ch:
				// we got a new event
				assert.NotNil(t, event)
			case <-ctx.Done():
				// our timeout is fired
				// this should close our channel
				listener.CloseChaincodeEvents()
				return
			}
		}
	}()

	assert.Eventually(t, func() bool {
		// eventually our event should be closed
		_, ok := <-ch
		return !ok
	}, waitFor, tick)

	// now we let our consumer know that they can stop working
	close(done)
	wg.Wait()
}

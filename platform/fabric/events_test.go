/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	publicationSnooze = time.Millisecond
	timeout           = 75 * time.Millisecond
	waitFor           = 4 * timeout
	tick              = timeout / 10
	maxVal            = 1000
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
	l := m.listener
	m.m.RUnlock()

	if l != nil {
		l.OnReceive(event)
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

	require.Eventually(t, func() bool {
		// eventually our event should be closed
		_, ok := <-ch
		return !ok
	}, waitFor, tick)

	// now we let our consumer know that they can stop working
	close(done)
	wg.Wait()
}

func TestEventServiceMultipleClose(t *testing.T) {
	subscriber := &mockSubscriber{}
	listener := newEventListener(subscriber, "testChaincode")
	_ = listener.ChaincodeEvents()
	msg1 := &committer.ChaincodeEvent{Payload: []byte("msg1")}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		subscriber.Publish("testChaincode", msg1)
		listener.CloseChaincodeEvents()
	}()

	// Call Close multiple times safely
	listener.CloseChaincodeEvents()
	listener.CloseChaincodeEvents()
	listener.CloseChaincodeEvents()

	wg.Wait()
}

func TestEventListenerDeadlock(t *testing.T) {
	subscriber := &mockSubscriber{}
	listener := newEventListener(subscriber, "testChaincode")
	ch := listener.ChaincodeEvents()

	msg1 := &committer.ChaincodeEvent{Payload: []byte("msg1")}
	msg2 := &committer.ChaincodeEvent{Payload: []byte("msg2")}

	// we publish and then consume
	subscriber.Publish("testChaincode", msg1)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equal(c, msg1, <-ch)
	}, timeout, 10*time.Millisecond)

	// next up, we publish msg1 and spawn another goroutine to publish msg2
	subscriber.Publish("testChaincode", msg1)

	var wg sync.WaitGroup
	wg.Add(1)
	// another producer
	go func() {
		defer wg.Done()
		// as msg1 is not yet consumed, our produces is blocked
		subscriber.Publish("testChaincode", msg2)
	}()

	// let's give the producer a bit time
	runtime.Gosched()
	for {
		value := rand.Intn(maxVal)
		if value == 0 {
			break
		}
	}

	// let's make sure that our produce is still waiting to complete publish
	require.Never(t, func() bool {
		// we expect to be blocked
		wg.Wait()
		return false
	}, timeout, 10*time.Millisecond)

	// no we close the listener, which should unblock the producer
	listener.CloseChaincodeEvents()

	// wait for the producer to finish
	wg.Wait()

	// we expect msg1 to be successfully published
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equal(c, msg1, <-ch)
	}, timeout, 10*time.Millisecond)

	// msg2 should not be published
	require.Empty(t, ch)
}

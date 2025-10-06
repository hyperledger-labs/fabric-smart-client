/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
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
	longTimeout       = 1 * time.Minute
	waitFor           = 1 * timeout
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

	msg := &committer.ChaincodeEvent{Payload: []byte("some msg")}
	stopPublisher := make(chan bool)

	var wg sync.WaitGroup

	// Publish events
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopPublisher:
				return
			default:
				subscriber.Publish("testChaincode", msg)
				time.Sleep(publicationSnooze)
			}
		}
	}()

	// Stop the consumer and close the event listener while the producer is still publishing
	ctx, cancel := context.WithTimeout(context.Background(), waitFor)
	defer cancel()

	// Consumer
	wg.Add(1)
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

	// let's wait until our timeout is fired
	<-ctx.Done()

	// consume everything that is remaining in ch and eventually the channel should be closed
	require.Eventually(t, func() bool {
		_, ok := <-ch
		return !ok
	}, waitFor, tick)

	// now we let our publisher know that they can stop working
	close(stopPublisher)
	wg.Wait()

	// check that our channel is closed
	require.Eventually(t, func() bool {
		return isClosed(ch)
	}, timeout, tick)
}

func TestEventServiceMultipleClose(t *testing.T) {
	subscriber := &mockSubscriber{}
	listener := newEventListener(subscriber, "testChaincode")
	ch := listener.ChaincodeEvents()
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

	// check that our channel is closed
	require.Eventually(t, func() bool {
		return isClosed(ch)
	}, timeout, tick)
}

func TestEventListenerDeadlock(t *testing.T) {
	subscriber := &mockSubscriber{}

	const customBufferLen = 1

	// in this test we configure our event listener with a smaller buffer and long recvTimeout
	listener := &EventListener{
		chaincodeName: "testChaincode",
		subscriber:    subscriber,
		eventCh:       make(chan *committer.ChaincodeEvent, customBufferLen),
		middleCh:      make(chan *committer.ChaincodeEvent),
		closing:       make(chan struct{}),
		closed:        make(chan struct{}),
		recvTimeout:   longTimeout,
	}

	ch := listener.ChaincodeEvents()

	msg1 := &committer.ChaincodeEvent{Payload: []byte("msg1")}
	msg2 := &committer.ChaincodeEvent{Payload: []byte("msg2")}

	// we publish and then consume
	subscriber.Publish("testChaincode", msg1)
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		require.Len(ct, ch, 1)
		require.Equal(ct, msg1, <-ch)
		require.Len(ct, ch, 0)
	}, timeout, tick)

	// next up, we fill our event buffer by publishing msg1
	for range customBufferLen {
		subscriber.Publish("testChaincode", msg1)
	}
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		// out channel should be full now
		require.Len(ct, ch, customBufferLen)
	}, timeout, tick)

	require.Never(t, func() bool {
		// this should be blocking (until longTimeout is fired)
		subscriber.Publish("testChaincode", msg1)
		return false
	}, timeout, tick)

	// we kick off our producer to publish msg2
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// as msg1 is not yet consumed, our producer is blocked
		subscriber.Publish("testChaincode", msg2)
	}()

	// let's give the producer a bit time
	runtime.Gosched()
	time.Sleep(waitFor)

	// let's make sure that our producer is still waiting to complete publish msg2
	require.Never(t, func() bool {
		// we expect to be blocked
		wg.Wait()
		return false
	}, timeout, tick)

	// now, we close the listener, which should unblock the producer
	listener.CloseChaincodeEvents()

	// wait for the producer to finish
	wg.Wait()

	// we expect msg1 to be successfully published
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.Equal(c, msg1, <-ch)
	}, timeout, tick)

	// check that our channel is closed
	require.Eventually(t, func() bool {
		return isClosed(ch)
	}, timeout, tick)
}

func isClosed(ch <-chan *committer.ChaincodeEvent) bool {
	select {
	case <-ch:
		return true
	default:
	}
	return false
}

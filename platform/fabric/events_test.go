/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
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
	t.Parallel()
	subscriber := &mockSubscriber{}
	listener := newEventListener(subscriber, "testChaincode")
	ch := listener.ChaincodeEvents()

	msg := &committer.ChaincodeEvent{Payload: []byte("some msg")}
	stopPublisher := make(chan bool)

	var wg sync.WaitGroup

	// Publish events
	wg.Go(func() {
		for {
			select {
			case <-stopPublisher:
				return
			default:
				subscriber.Publish("testChaincode", msg)
				time.Sleep(publicationSnooze)
			}
		}
	})

	// Stop the consumer and close the event listener while the producer is still publishing
	ctx, cancel := context.WithTimeout(t.Context(), waitFor)
	t.Cleanup(cancel)

	// Consumer. It records how many events it saw and whether any were nil, so
	// the test goroutine (not this one) can assert on the outcome afterwards -
	// testify assertions must not run off the main test goroutine.
	var received atomic.Int64
	var sawNil atomic.Bool
	wg.Go(func() {
		for {
			select {
			case event := <-ch:
				// we got a new event
				if event == nil {
					sawNil.Store(true)
				} else {
					received.Add(1)
				}
			case <-ctx.Done():
				// our timeout is fired
				// this should close our channel
				listener.CloseChaincodeEvents()
				return
			}
		}
	})

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

	// the consumer ran on a separate goroutine, so assert its recorded outcome
	// here on the test goroutine: it must have seen events, none of them nil.
	require.False(t, sawNil.Load(), "consumer received a nil event")
	require.Positive(t, received.Load(), "consumer received no events")

	// check that our channel is closed
	require.Eventually(t, func() bool {
		return isClosed(ch)
	}, timeout, tick)
}

func TestEventServiceMultipleClose(t *testing.T) {
	t.Parallel()
	subscriber := &mockSubscriber{}
	listener := newEventListener(subscriber, "testChaincode")
	ch := listener.ChaincodeEvents()
	msg1 := &committer.ChaincodeEvent{Payload: []byte("msg1")}

	var wg sync.WaitGroup
	wg.Go(func() {
		subscriber.Publish("testChaincode", msg1)
		listener.CloseChaincodeEvents()
	})

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
	t.Parallel()
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
		require.Empty(ct, ch)
	}, timeout, tick)

	// next up, we fill our event buffer by publishing msg1
	for range customBufferLen {
		subscriber.Publish("testChaincode", msg1)
	}
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		// out channel should be full now
		require.Len(ct, ch, customBufferLen)
	}, timeout, tick)

	// The pipeline can hold one more event than eventCh's buffer: the forwarding
	// goroutine pulls an event off the unbuffered middleCh and then blocks handing
	// it to the (now full) eventCh. So this extra publish does NOT block - the
	// middleCh handshake succeeds, the event is retained inside the listener, and
	// Publish returns.
	var published atomic.Bool
	go func() {
		subscriber.Publish("testChaincode", msg1)
		published.Store(true)
	}()
	require.Eventually(t, published.Load, timeout, tick)

	// Now the pipeline is truly full (eventCh full + one event retained in the
	// forwarder), so the next producer blocks until the listener is closed.
	var published2 atomic.Bool
	var wg sync.WaitGroup
	wg.Go(func() {
		// the pipeline is full, so this producer stays blocked
		subscriber.Publish("testChaincode", msg2)
		published2.Store(true)
	})

	// let's make sure that our producer is still waiting to complete publish
	// msg2. require.Never polls for the whole timeout window, so it both gives
	// the producer time to run and asserts it stays blocked - no sleep needed.
	require.Never(t, published2.Load, timeout, tick)

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

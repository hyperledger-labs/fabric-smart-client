/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package simple

import (
	"fmt"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/fakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEvents(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Test simple event system")
}

var _ = Describe("Event system", func() {

	When("creating a notifier service", func() {
		It("should succeed", func() {
			notifier := NewEventBus()
			Expect(notifier).NotTo(BeNil())
			Expect(notifier.handlers).NotTo(BeNil())
			Expect(notifier)
			Expect(&notifier.lock).NotTo(BeNil())
		})
	})

	When("publish and subscribe", func() {
		var notifier *eventBus

		var alice events.Subscriber
		var bob events.Publisher

		var listener *fakes.Listener

		BeforeEach(func() {
			notifier = NewEventBus()
			alice = notifier
			bob = notifier
			Expect(notifier.handlers).To(BeEmpty())

			listener = &fakes.Listener{}
		})

		It("Subscribe", func() {
			alice.Subscribe("topicAAA", listener)
			Expect(notifier.handlers).ToNot(BeEmpty())

			alice.Unsubscribe("topicAAA", nil)
			Expect(notifier.handlers).ToNot(BeEmpty())

			alice.Unsubscribe("topicBBB", listener)
			Expect(notifier.handlers).ToNot(BeEmpty())

			alice.Unsubscribe("topicAAA", listener)
			Expect(notifier.handlers).To(BeEmpty())

			alice.Unsubscribe("topicAAA", nil)
			Expect(notifier.handlers).To(BeEmpty())

			alice.Subscribe("topicAAA", listener)
			Expect(notifier.handlers).ToNot(BeEmpty())

			listener2 := &fakes.Listener{}
			alice.Unsubscribe("topicAAA", listener2)
			Expect(notifier.handlers).ToNot(BeEmpty())
		})

		It("Publish", func() {
			event := &fakes.Event{}
			event.TopicReturns("topicAAA")
			event.MessageReturns("HelloWorld")

			bob.Publish(nil)
			Expect(listener.OnReceiveCallCount()).To(Equal(0))

			bob.Publish(event)
			Expect(event.TopicCallCount()).To(Equal(1))
			Expect(listener.OnReceiveCallCount()).To(Equal(0))

			alice.Subscribe("topicAAA", listener)
			Expect(notifier.handlers).ToNot(BeEmpty())

			for i := 0; i < 100000; i++ {
				bob.Publish(event)
				Expect(event.TopicCallCount()).To(Equal(i + 2))
				Expect(listener.OnReceiveCallCount()).To(Equal(i + 1))
			}
		})

		It("many events", func() {
			event := &fakes.Event{}
			event.TopicReturns("topicAAA")
			event.MessageReturns("HelloWorld")

			alice.Subscribe("topicAAA", listener)
			Expect(notifier.handlers).ToNot(BeEmpty())

			for i := 0; i < 100000; i++ {
				bob.Publish(event)
				Expect(event.TopicCallCount()).To(Equal(i + 1))
				Expect(listener.OnReceiveCallCount()).To(Equal(i + 1))
			}
		})

		It("many topics", func() {
			event := &fakes.Event{}
			event.TopicReturns("topicAAA")
			event.MessageReturns("HelloWorld")

			var listeners []*fakes.Listener

			for i := 0; i < 10000; i++ {
				l := &fakes.Listener{}
				alice.Subscribe(fmt.Sprintf("topic_%d", i), l)
				listeners = append(listeners, l)
				Expect(len(notifier.handlers)).To(Equal(i + 1))
			}
		})

		It("many listeners", func() {
			event := &fakes.Event{}
			event.TopicReturns("topicAAA")
			event.MessageReturns("HelloWorld")

			var listeners []*fakes.Listener

			for i := 0; i < 10000; i++ {
				l := &fakes.Listener{}
				alice.Subscribe("topic", l)
				listeners = append(listeners, l)
				Expect(len(notifier.handlers)).To(Equal(1))
				Expect(len(notifier.handlers["topic"])).To(Equal(i + 1))
			}
		})
	})
})

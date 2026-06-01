/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package simple

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/mock"
)

func TestEvents(t *testing.T) { //nolint:paralleltest
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
		var notifier *EventBus

		var alice events.Subscriber
		var bob events.Publisher

		var listener *mock.Listener

		BeforeEach(func() {
			notifier = NewEventBus()
			alice = notifier
			bob = notifier
			Expect(notifier.handlers).To(BeEmpty())

			listener = &mock.Listener{}
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

			listener2 := &mock.Listener{}
			alice.Unsubscribe("topicAAA", listener2)
			Expect(notifier.handlers).ToNot(BeEmpty())
		})

		It("Publish", func() {
			event := &mock.Event{}
			event.TopicReturns("topicAAA")
			event.MessageReturns("HelloWorld")

			bob.Publish(nil)
			Expect(listener.OnReceiveCallCount()).To(Equal(0))

			bob.Publish(event)
			Expect(event.TopicCallCount()).To(Equal(1))
			Expect(listener.OnReceiveCallCount()).To(Equal(0))

			alice.Subscribe("topicAAA", listener)
			Expect(notifier.handlers).ToNot(BeEmpty())

			for i := range 100000 {
				bob.Publish(event)
				Expect(event.TopicCallCount()).To(Equal(i + 2))
				Expect(listener.OnReceiveCallCount()).To(Equal(i + 1))
			}
		})

		It("many events", func() {
			event := &mock.Event{}
			event.TopicReturns("topicAAA")
			event.MessageReturns("HelloWorld")

			alice.Subscribe("topicAAA", listener)
			Expect(notifier.handlers).ToNot(BeEmpty())

			for i := range 100000 {
				bob.Publish(event)
				Expect(event.TopicCallCount()).To(Equal(i + 1))
				Expect(listener.OnReceiveCallCount()).To(Equal(i + 1))
			}
		})

		It("many topics", func() {
			event := &mock.Event{}
			event.TopicReturns("topicAAA")
			event.MessageReturns("HelloWorld")
			for i := range 10000 {
				l := &mock.Listener{}
				alice.Subscribe(fmt.Sprintf("topic_%d", i), l)
				Expect(len(notifier.handlers)).To(Equal(i + 1))
			}
		})

		It("many listeners", func() {
			event := &mock.Event{}
			event.TopicReturns("topicAAA")
			event.MessageReturns("HelloWorld")
			for i := range 10000 {
				l := &mock.Listener{}
				alice.Subscribe("topic", l)
				Expect(len(notifier.handlers)).To(Equal(1))
				Expect(len(notifier.handlers["topic"])).To(Equal(i + 1))
			}
		})
	})
})

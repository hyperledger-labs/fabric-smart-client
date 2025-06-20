/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEvents(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Event service provider test")
}

func newService() *events.Service {
	return &events.Service{EventSystem: simple.NewEventBus()}
}

var _ = Describe("Event system", func() {
	var r *registry.ServiceProvider

	BeforeEach(func() {
		r = registry.New()
	})

	When("creating a notifier Service", func() {
		It("should succeed", func() {
			notifierService := newService()
			Expect(notifierService).ShouldNot(BeNil())
			Expect(notifierService.GetPublisher()).ShouldNot(BeNil())
			Expect(notifierService.GetSubscriber()).ShouldNot(BeNil())
		})
	})

	When("getting notifier Service through Service provider", func() {
		var notifier *events.Service

		BeforeEach(func() {
			notifier = newService()
			err := r.RegisterService(notifier)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tickets_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/views"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// keepRunningEnv starts the demo network for clicking through by hand; see
// `make run-examples-tickets`.
const keepRunningEnv = "TICKETS_KEEP_RUNNING"

var _ = Describe("Demo", Label("keep-running"), Ordered, func() {
	s := NewTestSuite(nwofsc.WebSocket)
	BeforeAll(func() {
		if os.Getenv(keepRunningEnv) != "1" {
			Skip(keepRunningEnv + " is not 1")
		}
		s.Setup()
	})
	AfterAll(func() {
		if s.II != nil {
			s.TearDown()
		}
	})

	It("keeps the network running", func(ctx SpecContext) {
		s.create(states.Event{Name: "Neon Nights", Artist: "Neon", Venue: "Berlin", Date: "2027-03-02", Seats: 40, Price: 50})
		s.create(states.Event{Name: "Neon Nights", Artist: "Neon", Venue: "New York", Date: "2027-04-10", Seats: 40, Price: 60})

		GinkgoWriter.Println("\nThe ticket demo is running. Open each node's UI in its own window:")
		for _, node := range append([]string{views.PromoterNode}, tickets.Resellers...) {
			c := newRESTClient(s.II, node)
			// The listener binds 0.0.0.0; browsers and curl reach it as localhost.
			c.URL = strings.Replace(c.URL, "0.0.0.0", "localhost", 1)
			GinkgoWriter.Printf("\n  %-12s %s/\n", node, c.URL)
			for _, r := range s.II.FscPlatform.Peers {
				if r.Name == node {
					dir := s.II.FscPlatform.NodeLocalTLSDir(r.Peer)
					GinkgoWriter.Printf("  %-12s curl --cacert %s --cert %s --key %s %s/api/events\n", "",
						filepath.Join(dir, "ca.crt"), filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key"), c.URL)
				}
			}
		}
		GinkgoWriter.Println("\nPress Ctrl-C to stop the network.")
		// Ginkgo cancels the spec's context on Ctrl-C, then AfterAll stops the network.
		<-ctx.Done()
	})
})

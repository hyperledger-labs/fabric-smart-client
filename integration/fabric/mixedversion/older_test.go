/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixedversion_test

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/mixedversion"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// olderRelease is an FSC release older than version negotiation, which supports only version
// 1 of every component, such as v0.27.0: a module version or the absolute path of a checkout.
// The Makefile sets it. A release that negotiates, v0.28 or later, fails the specs: its nodes
// state their versions and fail with other errors, or share version 2 with the current tree.
var olderRelease = os.Getenv("FSC_OLDER_RELEASE")

// refusalTimeout bounds how long a refused session may take to fail. It is well below the
// minute a ping round waits for its answer, so a session that hangs instead of being refused
// fails the spec.
const refusalTimeout = 20 * time.Second

// A node of the current tree supports only version 2 of fabric and fabricx, and a node of
// olderRelease only version 1, so the two have no common version. Whichever opens the session,
// it fails at once with an error that says so, instead of misreading a message or waiting for
// an answer.
var _ = Describe("A release without version negotiation", Label(fsc.WebSocket), func() {
	BeforeEach(func() { Expect(olderRelease).NotTo(BeEmpty(), "set FSC_OLDER_RELEASE") })

	// A slice, not a map: Ginkgo needs the same spec tree on every build of it.
	for _, tc := range []struct{ older, refusal string }{
		// The older initiator states no versions; the responder takes them as version 1
		// and rejects the session, and the initiator returns the rejection.
		{"initiator", "no_common_version: fabric 1 not in [2, 2]"},
		// The older responder sends no preamble, so the initiator finds no common version
		// and sends nothing.
		{"responder", "component [fabric]: local [2, 2], peer [1, 1]: no common version"},
	} {
		Describe("as the "+tc.older, func() {
			s := newSuiteAt(mixedversion.PingPongTopology, olderRelease, tc.older)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)

			It("is refused with no common version", func() {
				start := time.Now()
				_, err := s.II.Client("initiator").CallView("init", nil)
				Expect(err).To(MatchError(ContainSubstring(tc.refusal)))
				Expect(time.Since(start)).To(BeNumerically("<", refusalTimeout), "the session was not refused at once")
			})
		})
	}
})

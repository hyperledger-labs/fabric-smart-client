/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fsc

import (
	"bytes"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	fooinitiator "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/foo/initiator"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/initiator"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/responder"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/registry"
)

var _ = Describe("EndToEnd", func() {
	Describe("generate main", func() {
		It("should not fail", func() {
			p := NewPlatform(&registry.Registry{
				TopologiesByName: map[string]nwo.Topology{TopologyName: NewTopology()},
			}, nil)

			n := NewNode("test")
			n.RegisterViewFactory("initiator", &initiator.Factory{})
			n.RegisterViewFactory("initiator", &fooinitiator.Factory{})
			n.RegisterViewFactory("responder", &responder.Factory{})
			n.RegisterViewFactory("initiator2", &initiator.Factory{})
			n.RegisterResponder(&responder.Responder{}, &initiator.Initiator{})
			buf := bytes.NewBuffer(nil)
			p.GenerateCmd(buf, n)

			ExpectedMainOne, err := ioutil.ReadFile("./testdata/main/main.go.output")
			Expect(err).ToNot(HaveOccurred())
			Expect(string(buf.Bytes())).To(BeEquivalentTo(string(ExpectedMainOne)))
			// Expect(ioutil.WriteFile("./testdata/main/main.go.output", buf.Bytes(), 0777)).ToNot(HaveOccurred())
		})
	})
})

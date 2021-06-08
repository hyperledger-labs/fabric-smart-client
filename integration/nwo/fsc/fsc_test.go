/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fsc

import (
	"bytes"
	"context"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/registry"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/foo/initiator"
	initiator2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/initiator"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/responder"
)

type DummySDK struct {
}

func NewDummySDK() *DummySDK {
	return &DummySDK{}
}

func (d *DummySDK) Install() error {
	panic("implement me")
}

func (d *DummySDK) Start(ctx context.Context) error {
	panic("implement me")
}

var _ = Describe("EndToEnd", func() {
	Describe("generate main", func() {
		It("should not fail", func() {
			p := NewPlatform(&registry2.Registry{
				TopologiesByName: map[string]nwo.Topology{TopologyName: NewTopology()},
			}, nil)

			n := node.NewNode("test")
			n.AddSDK(&DummySDK{})
			n.RegisterViewFactory("initiator", &initiator2.Factory{})
			n.RegisterViewFactory("initiator", &initiator.Factory{})
			n.RegisterViewFactory("responder", &responder.Factory{})
			n.RegisterViewFactory("initiator2", &initiator2.Factory{})
			n.RegisterResponder(&responder.Responder{}, &initiator2.Initiator{})
			buf := bytes.NewBuffer(nil)
			p.GenerateCmd(buf, &node.Peer{
				Name:         "initiator",
				Organization: "fsc",
				Node:         n,
			})

			ExpectedMainOne, err := ioutil.ReadFile("./testdata/main/main.go.output")
			Expect(err).ToNot(HaveOccurred())
			Expect(string(buf.Bytes())).To(BeEquivalentTo(string(ExpectedMainOne)))
			// Expect(ioutil.WriteFile("./testdata/main/main.go.output", buf.Bytes(), 0777)).ToNot(HaveOccurred())
		})
	})
})

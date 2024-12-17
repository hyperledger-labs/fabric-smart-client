/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"bytes"
	"context"
	"os"

	context2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/mocks"
	mocks2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/mocks/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/foo/initiator"
	initiator2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/initiator"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/testdata/responder"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
			p := NewPlatform(context2.New("", 0, nil), NewTopology(), nil)

			n := node.NewNode("test")
			n.AddSDK(&DummySDK{})
			n.AddSDK(&mocks.SDK{})
			n.AddSDK(&mocks2.SDK{})
			n.RegisterViewFactory("initiator", &initiator2.Factory{})
			n.RegisterViewFactory("initiator", &initiator.Factory{})
			n.RegisterViewFactory("responder", &responder.Factory{})
			n.RegisterViewFactory("initiator2", &initiator2.Factory{})
			n.RegisterResponder(&responder.Responder{}, &initiator2.Initiator{})
			buf := bytes.NewBuffer(nil)
			p.GenerateCmd(buf, node.NewReplica(&node.Peer{
				Name:         "initiator",
				Organization: "fsc",
				Node:         n,
			}, "initiator.0"))

			ExpectedMainOne, err := os.ReadFile("./testdata/main/main.go.output")
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(BeEquivalentTo(string(ExpectedMainOne)))
			// Expect(os.WriteFile("./testdata/main/main.go.output", buf.Bytes(), 0777)).ToNot(HaveOccurred())
		})

		It("should not fail from template", func() {
			p := NewPlatform(context2.New("", 0, nil), NewTopology(), nil)

			template := node.NewNode("test")
			template.RegisterViewFactory("initiator", &initiator2.Factory{})
			template.RegisterViewFactory("initiator", &initiator.Factory{})
			template.RegisterViewFactory("responder", &responder.Factory{})
			template.RegisterViewFactory("initiator2", &initiator2.Factory{})
			template.RegisterResponder(&responder.Responder{}, &initiator2.Initiator{})

			n := node.NewNodeFromTemplate("test", template)
			n.AddSDK(&DummySDK{})
			n.AddSDK(&mocks.SDK{})
			n.AddSDK(&mocks2.SDK{})

			buf := bytes.NewBuffer(nil)
			p.GenerateCmd(buf, node.NewReplica(&node.Peer{
				Name:         "initiator",
				Organization: "fsc",
				Node:         n,
			}, "initiator.0"))

			ExpectedMainOne, err := os.ReadFile("./testdata/main/main.go.output")
			Expect(err).ToNot(HaveOccurred())
			Expect(buf.String()).To(BeEquivalentTo(string(ExpectedMainOne)))
			// Expect(os.WriteFile("./testdata/main/main.go.output", buf.Bytes(), 0777)).ToNot(HaveOccurred())
		})

	})
})

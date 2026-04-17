/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/iou"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

const (
	timeout  = 30 * time.Second
	interval = 1 * time.Second
)

var _ = Describe("EndToEnd", func() {
	for _, c := range []nwofsc.P2PCommunicationType{nwofsc.WebSocket} {
		Describe("IOU Life Cycle", Label(c), func() {
			s := NewTestSuite(c, integration.NoReplication)
			BeforeEach(s.Setup)
			AfterEach(s.TearDown)

			It("succeeded", s.TestSucceeded)
		})
	}
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.IOUPort.StartPortForNode(), "", iou.Topology(&iou.SDK{}, commType, nodeOpts)...)
		if err != nil {
			return nil, err
		}

		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())

		ii.Generate()

		return ii, nil
	})}
}

func (s *TestSuite) TestSucceeded() {
	// Verify namespace is present with initial version (version 0)
	CheckNamespaceExists(s.II, "iou", 0)

	InitApprover(s.II, "approver1")
	InitApprover(s.II, "approver2")

	// create an IOU with approver2 - should fail because the EP requires approver1
	By("creating iou with approver2 should fail")
	_, err := CreateIOU(s.II, "", 10, "approver2")
	Expect(err).To(HaveOccurred())

	By("creating iou with approver1 should work")
	iouState, err := CreateIOU(s.II, "", 10, "approver1")
	Expect(err).NotTo(HaveOccurred())

	CheckState(s.II, "borrower", iouState, 10)
	CheckState(s.II, "lender", iouState, 10)
}

func CheckNamespaceExists(ii *integration.Infrastructure, name string, version int) {
	fxPlatform := func(ii *integration.Infrastructure) *nwofabricx.Platform {
		for _, t := range ii.NWO.Platforms {
			if fx, ok := t.(*nwofabricx.Platform); ok {
				return fx
			}
		}
		return nil
	}

	exp := network.Namespace{Name: name, Version: version}

	// first we find out fabric-x platform
	fx := fxPlatform(ii)
	Expect(fx).NotTo(BeNil())
	Eventually(fx.Network.ListInstalledNames(), timeout, interval).Should(ContainElements(exp), "namespace '%s' should be present with version %d after update", name, version)
}

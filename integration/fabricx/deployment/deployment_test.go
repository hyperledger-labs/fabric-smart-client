/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deployment_test

import (
	"path"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/deployment"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

const (
	timeout  = 30 * time.Second
	interval = 1 * time.Second
)

var _ = Describe("EndToEnd", func() {
	Describe("fabricx Deployment Life Cycle", Label(nwofsc.WebSocket), func() {
		s := NewTestSuite(nwofsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		It("succeeded", s.TestSucceeded)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.IOUPort.StartPortForNode(), "", deployment.Topology(&deployment.SDK{}, commType, nodeOpts)...)
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
	CheckNamespaceExists(s.II, "simple", 0)

	// create an Deployment with approver2 - should fail because the EP requires approver1
	By("creating deployment with approver2 should fail")
	_, err := CallCreate(s.II, "Bob", 10, "approver2")
	Expect(err).To(HaveOccurred())

	By("creating deployment with approver1 should work")
	_, err = CallCreate(s.II, "Bob", 10, "approver1")
	Expect(err).NotTo(HaveOccurred())
	CheckState(s.II, "creator", []views.SomeObject{{Owner: "Bob", Value: 10}})

	// update the EP to require approver2
	By("update EP to approver2")
	endorserPKPath := path.Join(s.II.TestDir, "fabric.default/crypto/peerOrganizations/org2.example.com/users/approver2@org2.example.com/msp/signcerts/approver2@org2.example.com-cert.pem")
	UpdateNamespacePolicy(s.II, "simple", "threshold:"+endorserPKPath, 0)

	// Wait for namespace update transaction to be finalized and propagated
	// The namespace update is a transaction that needs to be committed and
	// the new endorsement policy needs to be available to all nodes
	By("waiting for namespace update to be finalized")
	// Verify namespace is present with updated version (version 1)
	CheckNamespaceExists(s.II, "simple", 1)

	// create an Deployment with approver2 - should succeed now
	By("creating another deployment with approver2 should work")
	_, err = CallCreate(s.II, "Alice", 20, "approver2")
	Expect(err).NotTo(HaveOccurred())
	CheckState(s.II, "creator", []views.SomeObject{{Owner: "Alice", Value: 20}})
}

func UpdateNamespacePolicy(ii *integration.Infrastructure, name, policy string, version int) {
	fx := nwofabricx.FxPlatform(ii)
	Expect(fx).NotTo(BeNil())

	c := &topology.ChannelChaincode{
		Chaincode: topology.Chaincode{
			Name:    name,
			Version: strconv.Itoa(version),
			Policy:  policy,
		},
		Channel: "testchannel",
	}

	fx.Network.UpdateNamespace(c)
}

func CheckNamespaceExists(ii *integration.Infrastructure, name string, version int) {
	exp := network.Namespace{Name: name, Version: version}

	// first we find out fabric-x platform
	fx := nwofabricx.FxPlatform(ii)
	Expect(fx).NotTo(BeNil())
	Eventually(fx.Network.ListInstalledNames(), timeout, interval).Should(ContainElements(exp), "namespace '%s' should be present with version %d after update", name, version)
}

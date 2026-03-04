/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deployment_test

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/deployment"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple/views"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/fxconfig"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	timeout  = 30 * time.Second
	interval = 1 * time.Second
)

var _ = Describe("EndToEnd", func() {
	for _, c := range []nwofsc.P2PCommunicationType{nwofsc.WebSocket} {
		Describe("fabricx Deployment Life Cycle", Label(c), func() {
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
		ii, err := integration.New(integration.IOUPort.StartPortForNode(), "", deployment.Topology(&deployment.SDK{}, commType, nodeOpts)...)
		if err != nil {
			return nil, err
		}

		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())

		ii.Generate()

		return ii, nil
	})}
}

func updateEP(s *TestSuite) {
	host := s.II.Ctx.HostByOrdererID("fabric.default", "OrdererOrg.orderer")
	port := s.II.Ctx.PortsByOrdererID("fabric.default", "OrdererOrg.orderer")["Listen"]

	command := &fxconfig.UpdateNamespace{
		NamespaceCommon: fxconfig.NamespaceCommon{
			Name:    "simple",
			Channel: "testchannel",
			MSPConfig: fxconfig.MSPConfig{
				ConfigPath: path.Join(s.II.TestDir, "fabric.default/crypto/peerOrganizations/org1.example.com/users/approver1@org1.example.com/msp"),
				LocalMspID: "Org1MSP",
			},
			OrdererConfig: fxconfig.OrdererConfig{
				Address: fmt.Sprintf("%s:%d", host, port),
				TLSConfig: fxconfig.TLSConfig{
					Enabled:   false, // FIXME
					RootCerts: []string{path.Join(s.II.TestDir, "fabric.default/crypto/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem")},
				},
			},
			EndorserPKPath: path.Join(s.II.TestDir, "fabric.default/crypto/peerOrganizations/org1.example.com/users/approver2@org1.example.com/msp/signcerts/approver2@org1.example.com-cert.pem"),
		},
		// this is the current version
		Version: 0,
	}

	cmd := common.NewCommand(fxconfig.CMDPath(), command)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	Expect(err).NotTo(HaveOccurred())
}

func (s *TestSuite) TestSucceeded() {
	// Verify namespace is present with initial version (version 0)
	CheckNamespaceExists(s.II, "simple", 0)

	// create an Deployment with approver2 - should fail because the EP requires approver1
	By("creating deployment with approver2 should fail")

	_, err := CreateDeployment(s.II, "Owner1", 10, "approver2")
	Expect(err).To(HaveOccurred())

	By("creating deployment with approver1 should work")
	deploymentState, err := CreateDeployment(s.II, "Owner2", 10, "approver1")
	Expect(err).NotTo(HaveOccurred())
	testObjects := []views.SomeObject{
		{
			Owner: "Owner2",
			Value: 10,
		},
	}
	CheckState(s.II, "creator", deploymentState, 10, testObjects)

	// update the EP to require approver2
	By("update EP to approver2")
	updateEP(s)

	// Wait for namespace update transaction to be finalized and propagated
	// The namespace update is a transaction that needs to be committed and
	// the new endorsement policy needs to be available to all nodes
	By("waiting for namespace update to be finalized")
	// Verify namespace is present with updated version (version 1)
	CheckNamespaceExists(s.II, "simple", 1)

	// create an Deployment with approver2 - should succeed now
	By("creating another deployment with approver2 should work")
	anotherDeploymentState, err := CreateDeployment(s.II, "Owner3", 20, "approver2")
	Expect(err).NotTo(HaveOccurred())

	testObjects2 := []views.SomeObject{
		{
			Owner: "Owner3",
			Value: 20,
		},
	}
	CheckState(s.II, "creator", anotherDeploymentState, 20, testObjects2)
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

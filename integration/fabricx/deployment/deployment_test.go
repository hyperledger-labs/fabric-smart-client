/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deployment_test

import (
	"fmt"
	"net"
	"os"
	"path"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/deployment"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
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

func UpdateNamespacePolicy(ii *integration.Infrastructure, policy string) {
	fx := fxPlatform(ii)
	Expect(fx).NotTo(BeNil())

	// set up our admin identity used by fxconfig
	adminMspID := fx.Network.Organization("Org1").MSPID
	adminMspDir := fx.Network.PeerUserMSPDir(fx.Network.PeersInOrg("Org1")[0], "Admin")

	ordererEndpoint := fx.Network.OrdererAddress(fx.Network.Orderers[0], fabric_network.ListenPort)

	// committer details
	committerNode := fx.Network.Peer("Org1", "SC")
	committerSidecarPort := fmt.Sprintf("%d", fx.Network.PeerPort(committerNode, fabric_network.ListenPort))
	notificationsEndpoint := net.JoinHostPort("localhost", committerSidecarPort)

	// setup our new endorser
	command := &fxconfig.UpdateNamespace{
		NamespaceCommon: fxconfig.NamespaceCommon{
			Name:    "simple",
			Channel: "testchannel",
			MSPConfig: fxconfig.MSPConfig{
				ConfigPath: adminMspDir,
				LocalMspID: adminMspID,
			},
			OrdererConfig: fxconfig.OrdererConfig{
				Address: ordererEndpoint,
				TLSConfig: fxconfig.TLSConfig{
					Enabled: false, // FIXME
					RootCerts: []string{
						fx.Network.OrgOrdererTLSCACertificatePath(fx.Network.Organizations[0]),
					},
				},
			},
			NotificationsConfig: fxconfig.NotificationsConfig{
				Address:   notificationsEndpoint,
				TLSConfig: fxconfig.TLSConfig{},
			},
			Policy: policy,
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
	_, err := CallCreate(s.II, "Bob", 10, "approver2")
	Expect(err).To(HaveOccurred())

	By("creating deployment with approver1 should work")
	_, err = CallCreate(s.II, "Bob", 10, "approver1")
	Expect(err).NotTo(HaveOccurred())
	CheckState(s.II, "creator", []views.SomeObject{{Owner: "Bob", Value: 10}})

	// update the EP to require approver2
	By("update EP to approver2")
	endorserPKPath := path.Join(s.II.TestDir, "fabric.default/crypto/peerOrganizations/org2.example.com/users/approver2@org2.example.com/msp/signcerts/approver2@org2.example.com-cert.pem")
	UpdateNamespacePolicy(s.II, "threshold:"+endorserPKPath)

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

func CheckNamespaceExists(ii *integration.Infrastructure, name string, version int) {
	exp := network.Namespace{Name: name, Version: version}

	// first we find out fabric-x platform
	fx := fxPlatform(ii)
	Expect(fx).NotTo(BeNil())
	Eventually(fx.Network.ListInstalledNames(), timeout, interval).Should(ContainElements(exp), "namespace '%s' should be present with version %d after update", name, version)
}

func fxPlatform(ii *integration.Infrastructure) *nwofabricx.Platform {
	for _, t := range ii.NWO.Platforms {
		if fx, ok := t.(*nwofabricx.Platform); ok {
			return fx
		}
	}
	return nil
}

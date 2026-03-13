/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiendorsement_test

import (
	"fmt"
	"net"
	"os"
	// "path"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/multiendorsement"
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
	Describe("fabricx Multiendorsement Life Cycle", Label(nwofsc.WebSocket), func() {
		s := NewTestSuite(nwofsc.WebSocket, integration.NoReplication)
		BeforeEach(s.Setup)
		AfterEach(s.TearDown)

		It("merges endorsements from multiple approvers", s.TestMultiEndorsementMerge)
	})
})

type TestSuite struct {
	*integration.TestSuite
}

func NewTestSuite(commType nwofsc.P2PCommunicationType, nodeOpts *integration.ReplicationOptions) *TestSuite {
	return &TestSuite{integration.NewTestSuite(func() (*integration.Infrastructure, error) {
		ii, err := integration.New(integration.IOUPort.StartPortForNode(), "", multiendorsement.Topology(&multiendorsement.SDK{}, commType, nodeOpts)...)
		if err != nil {
			return nil, err
		}

		ii.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())

		ii.Generate()

		return ii, nil
	})}
}

func namespaceUpdateCommon(ii *integration.Infrastructure) fxconfig.NamespaceCommon {
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

	return fxconfig.NamespaceCommon{
		Name:    "simple",
		Channel: "testchannel",
		MSPConfig: fxconfig.MSPConfig{
			ConfigPath: adminMspDir,
			LocalMspID: adminMspID,
		},
		OrdererConfig: fxconfig.OrdererConfig{
			Address: ordererEndpoint,
			TLSConfig: fxconfig.TLSConfig{
				Enabled: false,
				RootCerts: []string{
					fx.Network.OrgOrdererTLSCACertificatePath(fx.Network.Organizations[0]),
				},
			},
		},
		NotificationsConfig: fxconfig.NotificationsConfig{
			Address:   notificationsEndpoint,
			TLSConfig: fxconfig.TLSConfig{},
		},
	}
}

func UpdateNamespacePolicyRequirements(ii *integration.Infrastructure, policy string, version int) {
	// Require one endorsement according to policy
	command := &fxconfig.UpdateNamespace{
		NamespaceCommon: func() fxconfig.NamespaceCommon {
			common := namespaceUpdateCommon(ii)
			common.Policy = policy
			return common
		}(),
		Version: version,
	}

	cmd := common.NewCommand(fxconfig.CMDPath(), command)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err := cmd.Run()
	Expect(err).NotTo(HaveOccurred())
}

func (s *TestSuite) TestMultiEndorsementMerge() {
	By("verifying namespace is present with initial version")
	CheckNamespaceExists(s.II, "simple", 0)
	
	By("updating namespace policy to require endorsements from Org1")
	UpdateNamespacePolicyRequirements(s.II, "AND('Org1MSP.member')", 0)
	By("waiting for namespace update to be finalized")
	CheckNamespaceExists(s.II, "simple", 1)

	By("creating endorsement with approver1 and policy = AND('Org1MSP.member') only should succeed")
	_, err := CallCreateWithApprovers(s.II, "Charlie", 30, "approver1")
	Expect(err).NotTo(HaveOccurred())

	By("updating namespace policy to require endorsements from Org1 and Org2 or Org3")
	UpdateNamespacePolicyRequirements(s.II, "OR(AND('Org1MSP.member','Org2MSP.member'),'Org3MSP.member')", 1)
	By("waiting for namespace update to be finalized")
	CheckNamespaceExists(s.II, "simple", 2)

	By("creating multiendorsement with approver1 only and policy = OR(AND('Org1MSP.member','Org2MSP.member'),'Org3MSP.member') should fail")
	_, err = CallCreateWithApprovers(s.II, "Alice", 33, "approver1")
	Expect(err).To(HaveOccurred())

	By("creating multiendorsement with approver2 only and policy = OR(AND('Org1MSP.member','Org2MSP.member'),'Org3MSP.member') should fail")
	_, err = CallCreateWithApprovers(s.II, "Alice", 33, "approver2")
	Expect(err).To(HaveOccurred())

	By("creating multiendorsement with both approver1 and approver2 and policy = OR(AND('Org1MSP.member','Org2MSP.member'),'Org3MSP.member') should work")
	_, err = CallCreateWithApprovers(s.II, "Alice", 33, "approver1", "approver2")
	Expect(err).NotTo(HaveOccurred())

	CheckState(s.II, "creator", []views.SomeObject{{Owner: "Alice", Value: 33}})

	By("creating multiendorsement with only approver3 and policy = OR(AND('Org1MSP.member','Org2MSP.member'),'Org3MSP.member') should work")
	_, err = CallCreateWithApprovers(s.II, "Bob", 40, "approver3")
	Expect(err).NotTo(HaveOccurred())

	CheckState(s.II, "creator", []views.SomeObject{{Owner: "Bob", Value: 40}})

	By("creating multiendorsement with all approver1 and approver2 and approver3 and policy = OR(AND('Org1MSP.member','Org2MSP.member'),'Org3MSP.member') should work")
	_, err = CallCreateWithApprovers(s.II, "Frank", 50, "approver1", "approver2", "approver3")
	Expect(err).NotTo(HaveOccurred())

	CheckState(s.II, "creator", []views.SomeObject{{Owner: "Frank", Value: 50}})
}

func CheckNamespaceExists(ii *integration.Infrastructure, name string, version int) {
	exp := network.Namespace{Name: name, Version: version}

	// first we find out fabric-x platform
	fx := fxPlatform(ii)
	Expect(fx).NotTo(BeNil())
	Eventually(fx.Network.ListInstalledNames, timeout, interval).Should(ContainElements(exp), "namespace '%s' should be present with version %d after update", name, version)
}

func fxPlatform(ii *integration.Infrastructure) *nwofabricx.Platform {
	for _, t := range ii.NWO.Platforms {
		if fx, ok := t.(*nwofabricx.Platform); ok {
			return fx
		}
	}
	return nil
}

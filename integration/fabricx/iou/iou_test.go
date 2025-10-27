/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou_test

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/fxconfig"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

func updateEP(s *TestSuite) {
	host := s.II.Ctx.HostByOrdererID("fabric.default", "OrdererOrg.orderer")
	port := s.II.Ctx.PortsByOrdererID("fabric.default", "OrdererOrg.orderer")["Listen"]

	command := &fxconfig.UpdateNamespace{
		NamespaceCommon: fxconfig.NamespaceCommon{
			Name:    "iou",
			Channel: "testchannel",
			MSPConfig: fxconfig.MSPConfig{
				Path: path.Join(s.II.TestDir, "fabric.default/crypto/peerOrganizations/org1.example.com/users/approver1@org1.example.com/msp"),
				Name: "Org1MSP",
			},
			OrdererConfig: fxconfig.OrdererConfig{
				Tls:      false, // TODO: fixme
				Endpoint: fmt.Sprintf("%s:%d", host, port),
				CAFile:   path.Join(s.II.TestDir, "fabric.default/crypto/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem"),
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
	//
	By("updating with approver2 should fail")
	UpdateIOU(s.II, iouState, 5, "approver2", "status is not valid [2]")

	// update the EP to require approver2
	By("update EP to approver2")
	updateEP(s)
	// TODO: wait for tx finality before continuing
	time.Sleep(5 * time.Second)

	// TODO: make this better can check for a specific namespace and version
	for _, t := range s.II.NWO.Platforms {
		fx, ok := t.(*nwofabricx.Platform)
		if !ok {
			continue
		}
		fx.Network.ListInstalledNames()
	}

	// create an IOU with approver2 - should succeed now
	By("creating another iou with approver2 should work")
	anotherIouState, err := CreateIOU(s.II, "", 20, "approver2")
	Expect(err).NotTo(HaveOccurred())

	_ = anotherIouState
	CheckState(s.II, "borrower", anotherIouState, 20)
	CheckState(s.II, "lender", anotherIouState, 20)

	// update with approver1 must fail now!
	By("updating with approver2 should fail")
	UpdateIOU(s.II, anotherIouState, 7, "approver1", "status is not valid [2]")

	By("updating with approver2 should work")
	UpdateIOU(s.II, anotherIouState, 7, "approver2")

	CheckState(s.II, "borrower", anotherIouState, 7)
	CheckState(s.II, "lender", anotherIouState, 7)
}

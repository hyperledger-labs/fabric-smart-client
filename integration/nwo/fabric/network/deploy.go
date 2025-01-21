/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer/lifecycle"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/gstruct"
)

func PackageAndInstallChaincode(n *Network, chaincode *topology.Chaincode, peers ...*topology.Peer) {
	// create temp file for chaincode package if not provided
	if chaincode.PackageFile == "" {
		tempFile, err := os.CreateTemp("", "chaincode-package")
		Expect(err).NotTo(HaveOccurred())
		tempFile.Close()
		defer os.Remove(tempFile.Name())
		chaincode.PackageFile = tempFile.Name()
	}

	// only create chaincode package if it doesn't already exist
	if _, err := os.Stat(chaincode.PackageFile); os.IsNotExist(err) {
		switch chaincode.Lang {
		case "binary":
			PackageChaincodeBinary(chaincode)
		default:
			packager := n.PackagerFactory()
			err := packager.PackageChaincode(chaincode.Path, chaincode.Lang, chaincode.Label, chaincode.PackageFile, nil)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// install on all peers
	InstallChaincode(n, chaincode, peers...)
}

func PackageChaincode(n *Network, chaincode *topology.Chaincode, peer *topology.Peer) {
	sess, err := n.PeerAdminSession(peer, commands.ChaincodePackage{
		NetworkPrefix: n.Prefix,
		Path:          chaincode.Path,
		Lang:          chaincode.Lang,
		Label:         chaincode.Label,
		OutputFile:    chaincode.PackageFile,
		ClientAuth:    n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}

func InstallChaincode(n *Network, chaincode *topology.Chaincode, peers ...*topology.Peer) {
	if chaincode.PackageID == "" {
		chaincode.SetPackageIDFromPackageFile()
	}

	for _, p := range peers {
		sess, err := n.PeerAdminSession(p, commands.ChaincodeInstall{
			NetworkPrefix: n.Prefix,
			PackageFile:   chaincode.PackageFile,
			ClientAuth:    n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		EnsureInstalled(n, chaincode.Label, chaincode.PackageID, p)
	}
}

func ApproveChaincodeForMyOrg(n *Network, channel string, orderer *topology.Orderer, chaincode *topology.Chaincode, peers ...*topology.Peer) {
	if chaincode.PackageID == "" {
		chaincode.SetPackageIDFromPackageFile()
	}

	// used to ensure we only approve once per org
	approvedOrgs := map[string]bool{}
	for _, p := range peers {
		if _, ok := approvedOrgs[p.Organization]; !ok {
			sess, err := n.PeerAdminSession(p, commands.ChaincodeApproveForMyOrg{
				NetworkPrefix:       n.Prefix,
				ChannelID:           channel,
				Orderer:             n.OrdererAddress(orderer, ListenPort),
				Name:                chaincode.Name,
				Version:             chaincode.Version,
				PackageID:           chaincode.PackageID,
				Sequence:            chaincode.Sequence,
				EndorsementPlugin:   chaincode.EndorsementPlugin,
				ValidationPlugin:    chaincode.ValidationPlugin,
				SignaturePolicy:     chaincode.SignaturePolicy,
				ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
				InitRequired:        chaincode.InitRequired,
				CollectionsConfig:   chaincode.CollectionsConfig,
				ClientAuth:          n.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
			approvedOrgs[p.Organization] = true
			Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
		}
	}
}

func CheckCommitReadinessUntilReady(n *Network, channel string, chaincode *topology.Chaincode, checkOrgs []*topology.Organization, peers ...*topology.Peer) {
	for _, p := range peers {
		keys := gstruct.Keys{}
		for _, org := range checkOrgs {
			keys[org.MSPID] = BeTrue()
		}
		Eventually(checkCommitReadiness(n, p, channel, chaincode), n.EventuallyTimeout).Should(gstruct.MatchKeys(gstruct.IgnoreExtras, keys))
	}
}

func CommitChaincode(n *Network, channel string, orderer *topology.Orderer, chaincode *topology.Chaincode, peer *topology.Peer, checkPeers ...*topology.Peer) {
	// commit using one peer per org
	commitOrgs := map[string]bool{}
	var peerAddresses []string
	for _, p := range checkPeers {
		if exists := commitOrgs[p.Organization]; !exists {
			peerAddresses = append(peerAddresses, n.PeerAddress(p, ListenPort))
			commitOrgs[p.Organization] = true
		}
	}

	sess, err := n.PeerAdminSession(peer, commands.ChaincodeCommit{
		NetworkPrefix:       n.Prefix,
		ChannelID:           channel,
		Orderer:             n.OrdererAddress(orderer, ListenPort),
		Name:                chaincode.Name,
		Version:             chaincode.Version,
		Sequence:            chaincode.Sequence,
		EndorsementPlugin:   chaincode.EndorsementPlugin,
		ValidationPlugin:    chaincode.ValidationPlugin,
		SignaturePolicy:     chaincode.SignaturePolicy,
		ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
		InitRequired:        chaincode.InitRequired,
		CollectionsConfig:   chaincode.CollectionsConfig,
		PeerAddresses:       peerAddresses,
		ClientAuth:          n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	for i := 0; i < len(peerAddresses); i++ {
		Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
	}
	checkOrgs := []*topology.Organization{}
	for org := range commitOrgs {
		checkOrgs = append(checkOrgs, n.Organization(org))
	}
	EnsureChaincodeCommitted(n, channel, chaincode.Name, chaincode.Version, chaincode.Sequence, checkOrgs, checkPeers...)
}

// EnsureChaincodeCommitted polls each supplied peer until the chaincode definition
// has been committed to the peer's rwset.
func EnsureChaincodeCommitted(n *Network, channel, name, version, sequence string, checkOrgs []*topology.Organization, peers ...*topology.Peer) {
	for _, p := range peers {
		sequenceInt, err := strconv.ParseInt(sequence, 10, 64)
		Expect(err).NotTo(HaveOccurred())
		approvedKeys := gstruct.Keys{}
		for _, org := range checkOrgs {
			approvedKeys[org.MSPID] = BeTrue()
		}
		Eventually(listCommitted(n, p, channel, name), n.EventuallyTimeout).Should(
			gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Version":   Equal(version),
				"Sequence":  Equal(sequenceInt),
				"Approvals": gstruct.MatchKeys(gstruct.IgnoreExtras, approvedKeys),
			}),
		)
	}
}

func InitChaincode(n *Network, channel string, orderer *topology.Orderer, chaincode *topology.Chaincode, peers ...*topology.Peer) {
	// init using one peer per org
	initOrgs := map[string]bool{}
	var peerAddresses []string
	for _, p := range peers {
		if exists := initOrgs[p.Organization]; !exists {
			peerAddresses = append(peerAddresses, n.PeerAddress(p, ListenPort))
			initOrgs[p.Organization] = true
		}
	}

	sess, err := n.PeerUserSession(peers[0], "User1", commands.ChaincodeInvoke{
		NetworkPrefix: n.Prefix,
		ChannelID:     channel,
		Orderer:       n.OrdererAddress(orderer, ListenPort),
		Name:          chaincode.Name,
		Ctor:          chaincode.Ctor,
		PeerAddresses: peerAddresses,
		WaitForEvent:  true,
		IsInit:        true,
		ClientAuth:    n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	for i := 0; i < len(peerAddresses); i++ {
		Eventually(sess.Err, n.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
	}
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))
}

func EnsureInstalled(n *Network, label, packageID string, peers ...*topology.Peer) {
	for _, p := range peers {
		Eventually(QueryInstalled(n, p), n.EventuallyTimeout).Should(
			ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
				gstruct.Fields{
					"Label":     Equal(label),
					"PackageId": Equal(packageID),
				},
			)),
		)
	}
}

func QueryInstalledReferences(n *Network, channel, label, packageID string, checkPeer *topology.Peer, nameVersions ...[]string) {
	chaincodes := make([]*lifecycle.QueryInstalledChaincodesResult_Chaincode, len(nameVersions))
	for i, nameVersion := range nameVersions {
		chaincodes[i] = &lifecycle.QueryInstalledChaincodesResult_Chaincode{
			Name:    nameVersion[0],
			Version: nameVersion[1],
		}
	}

	Expect(QueryInstalled(n, checkPeer)()).To(
		ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras,
			gstruct.Fields{
				"Label":     Equal(label),
				"PackageId": Equal(packageID),
				"References": HaveKeyWithValue(channel, gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras,
					gstruct.Fields{
						"Chaincodes": ConsistOf(chaincodes),
					},
				))),
			},
		)),
	)
}

type queryInstalledOutput struct {
	InstalledChaincodes []lifecycle.QueryInstalledChaincodesResult_InstalledChaincode `json:"installed_chaincodes"`
}

func QueryInstalled(n *Network, peer *topology.Peer) func() []lifecycle.QueryInstalledChaincodesResult_InstalledChaincode {
	return func() []lifecycle.QueryInstalledChaincodesResult_InstalledChaincode {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeQueryInstalled{
			NetworkPrefix: n.Prefix,
			ClientAuth:    n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		output := &queryInstalledOutput{}
		err = json.Unmarshal(sess.Out.Contents(), output)
		Expect(err).NotTo(HaveOccurred())
		return output.InstalledChaincodes
	}
}

type checkCommitReadinessOutput struct {
	Approvals map[string]bool `json:"approvals"`
}

func checkCommitReadiness(n *Network, peer *topology.Peer, channel string, chaincode *topology.Chaincode) func() map[string]bool {
	return func() map[string]bool {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeCheckCommitReadiness{
			NetworkPrefix:       n.Prefix,
			ChannelID:           channel,
			Name:                chaincode.Name,
			Version:             chaincode.Version,
			Sequence:            chaincode.Sequence,
			EndorsementPlugin:   chaincode.EndorsementPlugin,
			ValidationPlugin:    chaincode.ValidationPlugin,
			SignaturePolicy:     chaincode.SignaturePolicy,
			ChannelConfigPolicy: chaincode.ChannelConfigPolicy,
			InitRequired:        chaincode.InitRequired,
			CollectionsConfig:   chaincode.CollectionsConfig,
			ClientAuth:          n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
		output := &checkCommitReadinessOutput{}
		err = json.Unmarshal(sess.Out.Contents(), output)
		Expect(err).NotTo(HaveOccurred())
		return output.Approvals
	}
}

type queryCommittedOutput struct {
	Sequence  int64           `json:"sequence"`
	Version   string          `json:"version"`
	Approvals map[string]bool `json:"approvals"`
}

// listCommitted returns the result of the queryCommitted command.
// If the command fails for any reason (e.g. namespace not defined
// or a database access issue), it will return an empty output object.
func listCommitted(n *Network, peer *topology.Peer, channel, name string) func() queryCommittedOutput {
	return func() queryCommittedOutput {
		sess, err := n.PeerAdminSession(peer, commands.ChaincodeListCommitted{
			NetworkPrefix: n.Prefix,
			ChannelID:     channel,
			Name:          name,
			ClientAuth:    n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
		output := &queryCommittedOutput{}
		if sess.ExitCode() == 1 {
			// don't try to unmarshal the output as JSON if the query failed
			return *output
		}
		err = json.Unmarshal(sess.Out.Contents(), output)
		Expect(err).NotTo(HaveOccurred())
		return *output
	}
}

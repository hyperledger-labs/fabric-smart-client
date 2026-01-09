/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	protosorderer "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

// GetConfigBlock retrieves the current config block for a channel.
func GetConfigBlock(n *Network, peer *topology.Peer, orderer *topology.Orderer, channel string) *common.Block {
	tempDir, err := os.MkdirTemp(filepath.Join(n.Context.RootDir(), n.Prefix), "getConfigBlock")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer utils.IgnoreErrorWithOneArg(os.RemoveAll, tempDir)

	// fetch the config block
	output := filepath.Join(tempDir, "config_block.pb")
	sess, err := n.OrdererAdminSession(orderer, peer, commands.ChannelFetch{
		NetworkPrefix: n.Prefix,
		ChannelID:     channel,
		Block:         "config",
		Orderer:       n.OrdererAddress(orderer, ListenPort),
		OutputFile:    output,
		ClientAuth:    n.ClientAuthRequired,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	gomega.Expect(sess.Err).To(gbytes.Say("Received block: "))

	// unmarshal the config block bytes
	configBlock := UnmarshalBlockFromFile(output)
	return configBlock
}

// GetConfig retrieves the last config of the given channel.
func GetConfig(n *Network, peer *topology.Peer, orderer *topology.Orderer, channel string) *common.Config {
	configBlock := GetConfigBlock(n, peer, orderer, channel)
	// unmarshal the envelope bytes
	envelope, err := protoutil.GetEnvelopeFromBlock(configBlock.Data.Data[0])
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// unmarshal the payload bytes
	payload, err := protoutil.UnmarshalPayload(envelope.Payload)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// unmarshal the config envelope bytes
	configEnv := &common.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, configEnv)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// clone the config
	return configEnv.Config
}

// UpdateConfig computes, signs, and submits a configuration update and waits
// for the update to complete.
func UpdateConfig(n *Network, orderer *topology.Orderer, channel string, current, updated *common.Config, getConfigBlockFromOrderer bool, submitter *topology.Peer, additionalSigners ...*topology.Peer) {
	tempDir, err := os.MkdirTemp("", "updateConfig")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer utils.IgnoreErrorWithOneArg(os.RemoveAll, tempDir)

	// compute update
	configUpdate, err := Compute(current, updated)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	configUpdate.ChannelId = channel

	signedEnvelope, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil, // local signer
		&common.ConfigUpdateEnvelope{ConfigUpdate: protoutil.MarshalOrPanic(configUpdate)},
		0, // message version
		0, // epoch
	)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(signedEnvelope).NotTo(gomega.BeNil())

	updateFile := filepath.Join(tempDir, "update.pb")
	err = os.WriteFile(updateFile, protoutil.MarshalOrPanic(signedEnvelope), 0600)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	for _, signer := range additionalSigners {
		sess, err := n.PeerAdminSession(signer, commands.SignConfigTx{
			NetworkPrefix: n.Prefix,
			File:          updateFile,
			ClientAuth:    n.ClientAuthRequired,
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	var currentBlockNumber uint64
	// get current configuration block number
	if getConfigBlockFromOrderer {
		currentBlockNumber = CurrentConfigBlockNumber(n, submitter, orderer, channel)
	} else {
		currentBlockNumber = CurrentConfigBlockNumber(n, submitter, nil, channel)
	}

	sess, err := n.PeerAdminSession(submitter, commands.ChannelUpdate{
		NetworkPrefix: n.Prefix,
		ChannelID:     channel,
		Orderer:       n.OrdererAddress(orderer, ListenPort),
		File:          updateFile,
		ClientAuth:    n.ClientAuthRequired,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	gomega.Expect(sess.Err).To(gbytes.Say("Successfully submitted channel update"))

	if getConfigBlockFromOrderer {
		ccb := func() uint64 { return CurrentConfigBlockNumber(n, submitter, orderer, channel) }
		gomega.Eventually(ccb, n.EventuallyTimeout).Should(gomega.BeNumerically(">", currentBlockNumber))
		return
	}
	// wait for the block to be committed to all peers that
	// have joined the channel
	for _, peer := range n.PeersWithChannel(channel) {
		ccb := func() uint64 { return CurrentConfigBlockNumber(n, peer, nil, channel) }
		gomega.Eventually(ccb, n.EventuallyTimeout).Should(gomega.BeNumerically(">", currentBlockNumber))
	}
}

// CurrentConfigBlockNumber retrieves the block number from the header of the
// current config block. This can be used to detect when configuration change
// has completed. If an orderer is not provided, the current config block will
// be fetched from the peer.
func CurrentConfigBlockNumber(n *Network, peer *topology.Peer, orderer *topology.Orderer, channel string) uint64 {
	tempDir, err := os.MkdirTemp(filepath.Join(n.Context.RootDir(), n.Prefix), "currentConfigBlock")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer utils.IgnoreErrorWithOneArg(os.RemoveAll, tempDir)

	// fetch the config block
	output := filepath.Join(tempDir, "config_block.pb")
	if orderer == nil {
		return CurrentConfigBlockNumberFromPeer(n, peer, channel, output)
	}

	FetchConfigBlock(n, peer, orderer, channel, output)

	// unmarshal the config block bytes
	configBlock := UnmarshalBlockFromFile(output)

	return configBlock.Header.Number
}

// CurrentConfigBlockNumberFromPeer retrieves the block number from the header
// of the peer's current config block.
func CurrentConfigBlockNumberFromPeer(n *Network, peer *topology.Peer, channel, output string) uint64 {
	sess, err := n.PeerAdminSession(peer, commands.ChannelFetch{
		NetworkPrefix: n.Prefix,
		ChannelID:     channel,
		Block:         "config",
		OutputFile:    output,
		ClientAuth:    n.ClientAuthRequired,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	gomega.Expect(sess.Err).To(gbytes.Say("Received block: "))

	configBlock := UnmarshalBlockFromFile(output)

	return configBlock.Header.Number
}

// FetchConfigBlock fetches latest config block.
func FetchConfigBlock(n *Network, peer *topology.Peer, orderer *topology.Orderer, channel string, output string) {
	fetch := func() int {
		sess, err := n.OrdererAdminSession(orderer, peer, commands.ChannelFetch{
			NetworkPrefix: n.Prefix,
			ChannelID:     channel,
			Block:         "config",
			Orderer:       n.OrdererAddress(orderer, ListenPort),
			OutputFile:    output,
			ClientAuth:    n.ClientAuthRequired,
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		code := sess.Wait(n.EventuallyTimeout).ExitCode()
		if code == 0 {
			gomega.Expect(sess.Err).To(gbytes.Say("Received block: "))
		}
		return code
	}
	gomega.Eventually(fetch, n.EventuallyTimeout).Should(gomega.Equal(0))
}

// UpdateOrdererConfig computes, signs, and submits a configuration update
// which requires orderers signature and waits for the update to complete.
func UpdateOrdererConfig(n *Network, orderer *topology.Orderer, channel string, current, updated *common.Config, submitter *topology.Peer, additionalSigners ...*topology.Orderer) {
	tempDir, err := os.MkdirTemp(filepath.Join(n.Context.RootDir(), n.Prefix), "updateConfig")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	updateFile := filepath.Join(tempDir, "update.pb")
	defer utils.IgnoreErrorWithOneArg(os.RemoveAll, tempDir)

	currentBlockNumber := CurrentConfigBlockNumber(n, submitter, orderer, channel)
	ComputeUpdateOrdererConfig(updateFile, n, channel, current, updated, submitter, additionalSigners...)

	gomega.Eventually(func() bool {
		sess, err := n.OrdererAdminSession(orderer, submitter, commands.ChannelUpdate{
			NetworkPrefix: n.Prefix,
			ChannelID:     channel,
			Orderer:       n.OrdererAddress(orderer, ListenPort),
			File:          updateFile,
			ClientAuth:    n.ClientAuthRequired,
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		sess.Wait(n.EventuallyTimeout)
		if sess.ExitCode() != 0 {
			return false
		}

		return strings.Contains(string(sess.Err.Contents()), "Successfully submitted channel update")
	}, n.EventuallyTimeout).Should(gomega.BeTrue())

	// wait for the block to be committed
	ccb := func() uint64 { return CurrentConfigBlockNumber(n, submitter, orderer, channel) }
	gomega.Eventually(ccb, n.EventuallyTimeout).Should(gomega.BeNumerically(">", currentBlockNumber))
}

// UpdateOrdererConfigSession computes, signs, and submits a configuration
// update which requires orderer signatures. The caller should wait on the
// returned seession retrieve the exit code.
func UpdateOrdererConfigSession(n *Network, orderer *topology.Orderer, channel string, current, updated *common.Config, submitter *topology.Peer, additionalSigners ...*topology.Orderer) *gexec.Session {
	tempDir, err := os.MkdirTemp(filepath.Join(n.Context.RootDir(), n.Prefix), "updateConfig")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	updateFile := filepath.Join(tempDir, "update.pb")

	ComputeUpdateOrdererConfig(updateFile, n, channel, current, updated, submitter, additionalSigners...)

	// session should not return with a zero exit code nor with a success response
	sess, err := n.OrdererAdminSession(orderer, submitter, commands.ChannelUpdate{
		NetworkPrefix: n.Prefix,
		ChannelID:     channel,
		Orderer:       n.OrdererAddress(orderer, ListenPort),
		File:          updateFile,
		ClientAuth:    n.ClientAuthRequired,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return sess
}

func ComputeUpdateOrdererConfig(updateFile string, n *Network, channel string, current, updated *common.Config, submitter *topology.Peer, additionalSigners ...*topology.Orderer) {
	// compute update
	configUpdate, err := Compute(current, updated)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	configUpdate.ChannelId = channel

	signedEnvelope, err := protoutil.CreateSignedEnvelope(
		common.HeaderType_CONFIG_UPDATE,
		channel,
		nil, // local signer
		&common.ConfigUpdateEnvelope{ConfigUpdate: protoutil.MarshalOrPanic(configUpdate)},
		0, // message version
		0, // epoch
	)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(signedEnvelope).NotTo(gomega.BeNil())

	err = os.WriteFile(updateFile, protoutil.MarshalOrPanic(signedEnvelope), 0600)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	for _, signer := range additionalSigners {
		sess, err := n.OrdererAdminSession(signer, submitter, commands.SignConfigTx{
			NetworkPrefix: n.Prefix,
			File:          updateFile,
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

// UnmarshalBlockFromFile unmarshals a proto encoded block from a file.
func UnmarshalBlockFromFile(blockFile string) *common.Block {
	blockBytes, err := os.ReadFile(blockFile)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	block, err := protoutil.UnmarshalBlock(blockBytes)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return block
}

// ConsensusMetadataMutator receives ConsensusType.Metadata and mutates it.
type ConsensusMetadataMutator func([]byte) []byte

// MSPMutator receives FabricMSPConfig and mutates it.
type MSPMutator func(config *msp.FabricMSPConfig) *msp.FabricMSPConfig

// UpdateConsensusMetadata executes a config update that updates the consensus
// metadata according to the given ConsensusMetadataMutator.
func UpdateConsensusMetadata(network *Network, peer *topology.Peer, orderer *topology.Orderer, channel string, mutateMetadata ConsensusMetadataMutator) {
	config := GetConfig(network, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	consensusTypeConfigValue := updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	consensusTypeValue := &protosorderer.ConsensusType{}
	err := proto.Unmarshal(consensusTypeConfigValue.Value, consensusTypeValue)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	consensusTypeValue.Metadata = mutateMetadata(consensusTypeValue.Metadata)

	updatedConfig.ChannelGroup.Groups["Orderer"].Values["ConsensusType"] = &common.ConfigValue{
		ModPolicy: "Admins",
		Value:     protoutil.MarshalOrPanic(consensusTypeValue),
	}

	UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}

func UpdateOrdererMSP(network *Network, peer *topology.Peer, orderer *topology.Orderer, channel, orgID string, mutateMSP MSPMutator) {
	config := GetConfig(network, peer, orderer, channel)
	updatedConfig := proto.Clone(config).(*common.Config)

	// Unpack the MSP config
	rawMSPConfig := updatedConfig.ChannelGroup.Groups["Orderer"].Groups[orgID].Values["MSP"]
	mspConfig := &msp.MSPConfig{}
	err := proto.Unmarshal(rawMSPConfig.Value, mspConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	fabricConfig := &msp.FabricMSPConfig{}
	err = proto.Unmarshal(mspConfig.Config, fabricConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Mutate it as we are asked
	fabricConfig = mutateMSP(fabricConfig)

	// Wrap it back into the config
	mspConfig.Config = protoutil.MarshalOrPanic(fabricConfig)
	rawMSPConfig.Value = protoutil.MarshalOrPanic(mspConfig)

	UpdateOrdererConfig(network, orderer, channel, config, updatedConfig, peer, orderer)
}

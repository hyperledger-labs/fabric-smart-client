/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evm

import (
	"context"
	"math/big"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/chain"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// fundAmount is what GenerateArtifacts gives each Submitters node: 10 ETH, 10^19 wei.
var fundAmount = new(big.Int).Mul(big.NewInt(10), big.NewInt(1_000_000_000_000_000_000))

// addressFor returns the address GenerateArtifacts generated for the FSC node named name. A
// Submitters or Approvers entry that doesn't match any FSC node name (most likely a typo) would
// otherwise resolve to the zero address with no error: fundable, and acceptable as a contract
// approver, but silently wrong.
func addressFor(addresses map[string]client.Address, name string) (client.Address, error) {
	addr, ok := addresses[name]
	if !ok {
		return client.Address{}, errors.Errorf("evm nwo: [%s] is not the name of any FSC node", name)
	}

	return addr, nil
}

type platformFactory struct{}

// NewPlatformFactory returns the factory the integration infrastructure registers for the "evm"
// topology type.
func NewPlatformFactory() api.PlatformFactory { return &platformFactory{} }

func (*platformFactory) Name() string { return TopologyName }

func (*platformFactory) New(ctx api.Context, t api.Topology, _ api.Builder) api.Platform {
	topology, ok := t.(*Topology)
	if !ok {
		panic("evm nwo: the topology passed to the evm platform is not an evm topology")
	}

	return &Platform{ctx: ctx, topology: topology}
}

// Platform starts (or connects to) an EVM network, funds the Submitters nodes, deploys the IOU
// contract and renders every FSC node's evm extension, all during GenerateArtifacts.
type Platform struct {
	ctx      api.Context
	topology *Topology

	besu     *Besu // nil in external mode
	client   client.EVMClient
	chainID  *big.Int
	contract client.Address
	funder   *chain.Submitter
}

var (
	_ api.Platform        = (*Platform)(nil)
	_ api.PlatformFactory = (*platformFactory)(nil)
)

func (p *Platform) Name() string { return p.topology.Name() }
func (p *Platform) Type() string { return p.topology.Type() }

func (*Platform) GenerateConfigTree() {}

// GenerateArtifacts starts the EVM network, generates a key per FSC node, funds the Submitters
// nodes, deploys the IOU contract with the Approvers addresses, and renders every FSC node's evm
// extension so core.yaml has it before the FSC platform starts any node.
func (p *Platform) GenerateArtifacts() {
	ctx := context.Background()

	endpoint, funderKeyHex := p.startNetwork(ctx)

	rpc, err := client.NewJSONRPCClient(endpoint, nil)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	p.client = rpc

	funderKey, err := iou.ParseKeyHex(funderKeyHex)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	p.funder = &chain.Submitter{
		Client:        p.client,
		Key:           funderKey,
		ChainID:       p.chainID,
		GasMultiplier: p.topology.GasMultiplier,
	}

	fscTopology, ok := p.ctx.TopologyByName("fsc").(*fsc.Topology)
	gomega.Expect(ok).To(gomega.BeTrue(), "evm nwo: no fsc topology found")
	nodes := fscTopology.ListNodes()

	keysDir := filepath.Join(p.ctx.RootDir(), "evm", "keys")
	addresses := make(map[string]client.Address, len(nodes))
	keyPaths := make(map[string]string, len(nodes))
	for _, n := range nodes {
		identity, err := GenerateKey(keysDir, n.Name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		addresses[n.Name] = identity.Address
		keyPaths[n.Name] = identity.Path
	}

	for _, name := range p.topology.Submitters {
		addr, err := addressFor(addresses, name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		hash, err := p.funder.Send(ctx, &addr, fundAmount, nil)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = chain.WaitReceipt(ctx, p.client, hash, p.topology.PollInterval, p.topology.Timeout)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	approverAddrs := make([]client.Address, len(p.topology.Approvers))
	for i, name := range p.topology.Approvers {
		addr, err := addressFor(addresses, name)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		approverAddrs[i] = addr
	}
	deployHash, err := p.funder.Send(ctx, nil, nil, contract.DeployData(approverAddrs))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	receipt, err := chain.WaitReceipt(ctx, p.client, deployHash, p.topology.PollInterval, p.topology.Timeout)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(receipt.ContractAddress).NotTo(gomega.BeNil(), "evm nwo: the IOU deployment left no contract address")
	p.contract = *receipt.ContractAddress

	for _, n := range nodes {
		rendered, err := RenderExtension(NodeConfig{
			Endpoint:      endpoint,
			ChainID:       p.chainID.Int64(),
			Contract:      p.contract,
			KeyPath:       keyPaths[n.Name],
			GasMultiplier: p.topology.GasMultiplier,
			PollInterval:  p.topology.PollInterval,
			Timeout:       p.topology.Timeout,
		})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for _, uniqueName := range n.ReplicaUniqueNames() {
			p.ctx.AddExtension(uniqueName, Extension, rendered)
		}
	}
}

// startNetwork starts Besu, or checks the externally supplied endpoint's chain ID against
// EVM_CHAIN_ID, and returns the endpoint to use and the funder's hex-encoded private key.
func (p *Platform) startNetwork(ctx context.Context) (endpoint, funderKeyHex string) {
	external, ok, err := ExternalFromEnv()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	if ok {
		rpc, err := client.NewJSONRPCClient(external.Endpoint, nil)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		id, err := rpc.ChainID(ctx)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(id.Int64()).To(gomega.Equal(external.ChainID),
			"evm nwo: the external endpoint's chain id does not match EVM_CHAIN_ID")
		p.chainID = big.NewInt(external.ChainID)

		return external.Endpoint, external.FunderKey
	}

	besu, err := StartBesu(ctx, BesuConfig{Name: "besu-" + common.UniqueName(), Port: p.ctx.ReservePort()})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	p.besu = besu
	p.chainID = big.NewInt(DefaultChainID)

	return besu.Endpoint(), DevFundedKey
}

func (*Platform) Load() {}

// Members returns no long-running processes: the Besu container, if any, is managed directly by
// Cleanup rather than as a member of the test network's process group.
func (*Platform) Members() []grouper.Member { return nil }

func (*Platform) PostRun(bool) {}

// Cleanup stops the Besu container, if GenerateArtifacts started one.
func (p *Platform) Cleanup() {
	if p.besu == nil {
		return
	}
	if err := p.besu.Stop(context.Background()); err != nil {
		logger.Errorf("evm nwo: failed to stop besu: %s", err)
	}
}

// Contract returns the deployed IOU contract's address.
func (p *Platform) Contract() client.Address { return p.contract }

// Client returns the EVM client GenerateArtifacts built for the network's endpoint.
func (p *Platform) Client() client.EVMClient { return p.client }

// Funder returns the submitter GenerateArtifacts funded every Submitters node from.
func (p *Platform) Funder() *chain.Submitter { return p.funder }

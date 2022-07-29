/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"path/filepath"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

var (
	packageLock sync.Mutex
	packagePath string
	cleanupFunc = func() {}
)

func (p *Platform) InteropChaincodeFile() string {
	return filepath.Join(
		p.RelayDir(),
		"interop.tar.gz",
	)
}

func (p *Platform) interopccSetup(relay *RelayServer, cc *topology.ChannelChaincode) (*topology.ChannelChaincode, error) {
	cc.Chaincode.Ctor = `{"Args":["initLedger","applicationCCID"]}`

	packageLock.Lock()
	defer packageLock.Unlock()

	if packagePath != "" {
		cc.Chaincode.PackageFile = packagePath
		return cc, nil
	}

	path, cleanup, err := packageChaincode()
	if err != nil {
		return nil, err
	}

	cc.Chaincode.PackageFile = path

	packagePath = path
	cleanupFunc = cleanup

	return cc, nil
}

func (p *Platform) PrepareInteropChaincode(relay *RelayServer) (*topology.ChannelChaincode, error) {
	orgs := p.Fabric(relay).PeerOrgs()

	policy := "OR ("
	for i, org := range orgs {
		if i > 0 {
			policy += ","
		}
		policy += "'" + org.Name + "MSP.member'"
	}
	policy += ")"

	var peers []string
	for _, org := range orgs {
		for _, peer := range p.Fabric(relay).Topology().Peers {
			if peer.Organization == org.Name {
				peers = append(peers, peer.Name)
			}
		}
	}

	return p.interopccSetup(relay, &topology.ChannelChaincode{
		Chaincode: topology.Chaincode{
			Name:            relay.InteropChaincode.Namespace,
			Version:         "Version-0.0",
			Sequence:        "1",
			InitRequired:    true,
			Path:            relay.InteropChaincode.Path,
			Lang:            "golang",
			Label:           relay.InteropChaincode.Namespace,
			Policy:          policy,
			SignaturePolicy: policy,
		},
		Channel: relay.InteropChaincode.Channel,
		Peers:   peers,
	})
}

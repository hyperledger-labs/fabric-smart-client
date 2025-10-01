/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/extensions/scv2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/tedsuo/ifrit/grouper"
)

var logger = logging.MustGetLogger()

const PlatformName = "fabricx"

type ExtensionFactory func(platform *Platform, registry api.Context, t *topology.Topology, builder api.Builder) fabric_network.Extension

type ExtendedPlatformFactory struct {
	*PlatformFactory
	extensions []ExtensionFactory
}

func (f *ExtendedPlatformFactory) New(registry api.Context, t api.Topology, builder api.Builder) api.Platform {
	p := f.PlatformFactory.New(registry, t, builder)

	topo, ok := t.(*topology.Topology)
	if !ok {
		panic(fmt.Errorf("invalid topology type"))
	}

	for _, ext := range f.extensions {
		p.Network.AddExtension(ext(p, registry, topo, builder))
	}

	return p
}

func (f *ExtendedPlatformFactory) WithExtensions(exts ...ExtensionFactory) *ExtendedPlatformFactory {
	return &ExtendedPlatformFactory{
		PlatformFactory: f.PlatformFactory,
		extensions:      append(f.extensions, exts...),
	}
}

func NewPlatformFactory() *ExtendedPlatformFactory {
	return NewPlatformFactoryWithSidecar("", nil, "SC", "Org1")
}

func NewPlatformFactoryWithSidecar(sidecarHost string, sidecarPorts api.Ports, sidecarName, sidecarOrg string) *ExtendedPlatformFactory {
	return NewFabricxPlatformFactory().WithExtensions(
		func(platform *Platform, registry api.Context, t *topology.Topology, builder api.Builder) fabric_network.Extension {
			return scv2.NewExtension(t, platform.Network, sidecarHost, sidecarPorts, sidecarName, sidecarOrg)
		})
}

func NewFabricxPlatformFactory() *PlatformFactory {
	return &PlatformFactory{}
}

type PlatformFactory struct{}

func (f *PlatformFactory) WithExtensions(exts ...ExtensionFactory) *ExtendedPlatformFactory {
	return &ExtendedPlatformFactory{
		PlatformFactory: f,
		extensions:      exts,
	}
}

func (*PlatformFactory) Name() string {
	return PlatformName
}

func (*PlatformFactory) New(registry api.Context, t api.Topology, builder api.Builder) *Platform {
	topo, ok := t.(*topology.Topology)
	if !ok {
		utils.Must(errors.New("cannot cast topology"))
	}

	// create a new fabricx network
	n := network.New(
		registry,
		topo,
		builder,
		[]fabric_network.ChaincodeProcessor{},
		common.UniqueName(),
	)

	// create the platform
	p := &Platform{
		Network:       n,
		dockerSupport: &fabric.Docker{NetworkID: n.NetworkID},
	}

	return p
}

type Platform struct {
	Network       *network.Network
	dockerSupport *fabric.Docker
}

func (p *Platform) Name() string {
	return p.Network.Topology().TopologyName
}

func (p *Platform) Type() string {
	return p.Network.Topology().TopologyType
}

func (p *Platform) GenerateConfigTree() {
	p.Network.GenerateConfigTree()
}

func (p *Platform) GenerateArtifacts() {
	p.Network.GenerateArtifacts()
}

func (p *Platform) Load() {
	p.Network.Load()
}

func (p *Platform) Members() []grouper.Member {
	return p.Network.Members()
}

func (p *Platform) DeleteVault(id string) {
	logger.Warnf("Deleting vault for [%s]", id)
}

func (p *Platform) PostRun(load bool) {
	// set up our docker environment for chaincode containers
	err := p.dockerSupport.Setup()
	utils.Must(err)

	// network post run
	p.Network.PostRun(load)
}

func (p *Platform) Cleanup() {
	p.Network.Cleanup()

	// cleanup docker environment
	err := p.dockerSupport.Cleanup()
	utils.Must(err)
}

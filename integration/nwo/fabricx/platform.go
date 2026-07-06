/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"fmt"

	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/extensions/scv2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

var logger = logging.MustGetLogger()

const PlatformName = "fabricx"

type ExtensionFactory func(platform *Platform, registry api.Context, t api.Topology, builder api.Builder) fabric_network.Extension

type ExtendedPlatformFactory struct {
	*PlatformFactory
	extensions []ExtensionFactory
}

func (f *ExtendedPlatformFactory) New(registry api.Context, t api.Topology, builder api.Builder) api.Platform {
	p := f.PlatformFactory.New(registry, t, builder)

	for _, ext := range f.extensions {
		p.Network.AddExtension(ext(p, registry, t, builder))
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
	return NewFabricxPlatformFactory().WithExtensions(
		func(platform *Platform, registry api.Context, t api.Topology, builder api.Builder) fabric_network.Extension {
			fxTopo, ok := t.(*Topology)
			if !ok {
				panic(fmt.Sprintf("expected *fabricx.Topology, got %T", t))
			}
			return scv2.NewExtension(fxTopo.Topology, platform.Network, scv2.CommitterConfig{
				SidecarName:  fxTopo.Committer.Name,
				SidecarOrg:   fxTopo.Committer.Org,
				SidecarHost:  fxTopo.Committer.Host,
				SidecarPorts: fxTopo.Committer.Ports,
				Image:        fxTopo.Committer.Image,
				EnvVars:      fxTopo.Committer.EnvVars,
			})
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
	fxTopo, ok := t.(*Topology)
	if !ok {
		panic(fmt.Sprintf("expected *fabricx.Topology, got %T", t))
	}

	// create a new fabricx network
	n := network.New(
		registry,
		fxTopo.Topology,
		builder,
		[]fabric_network.ChaincodeProcessor{},
		common.UniqueName(),
		fxTopo.Committer.Org,
		fxTopo.Committer.Name,
	)

	// create the platform
	p := &Platform{
		Network:       n,
		dockerSupport: &fabric.Docker{NetworkID: n.NetworkID},
	}

	return p
}

func FxPlatform(ii *integration.Infrastructure) *Platform {
	for _, t := range ii.NWO.Platforms {
		if fx, ok := t.(*Platform); ok {
			return fx
		}
	}
	return nil
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
	utils.Must(p.dockerSupport.Setup())

	// network post run
	p.Network.PostRun(load)
}

func (p *Platform) Cleanup() {
	p.Network.Cleanup()

	// cleanup docker environment
	utils.Must(p.dockerSupport.Cleanup())
}

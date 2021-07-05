/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"io"
	"os"
	"path/filepath"
	"text/template"
	"time"

	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"

	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

type Builder interface {
	Build(path string) string
}

type FabricNetwork interface {
	DeployChaincode(chaincode *topology.ChannelChaincode)
	DefaultIdemixOrgMSPDir() string
	Topology() *topology.Topology
	PeerChaincodeAddress(peerName string) string
}

type platformFactory struct{}

func NewPlatformFactory() *platformFactory {
	return &platformFactory{}
}

func (f platformFactory) Name() string {
	return TopologyName
}

func (f platformFactory) New(registry api2.Context, t api2.Topology, builder api2.Builder) api2.Platform {
	return NewPlatform(registry, t, builder)
}

type Platform struct {
	Context           api2.Context
	Topology          *Topology
	Builder           api2.Builder
	EventuallyTimeout time.Duration

	colorIndex int
}

func NewPlatform(ctx api2.Context, t api2.Topology, builder api2.Builder) *Platform {
	return &Platform{
		Context:           ctx,
		Topology:          t.(*Topology),
		Builder:           builder,
		EventuallyTimeout: 10 * time.Minute,
	}
}

func (p *Platform) Name() string {
	return TopologyName
}

func (p *Platform) Type() string {
	return TopologyName
}

func (p *Platform) GenerateConfigTree() {
	for _, relay := range p.Topology.Relays {
		relay.Port = p.Context.ReservePort()
		for _, driver := range relay.Drivers {
			driver.Port = p.Context.ReservePort()
		}
	}
}

func (p *Platform) GenerateArtifacts() {
	for _, relay := range p.Topology.Relays {
		p.generateRelayServerTOML(relay)
	}
}

func (p *Platform) Load() {
}

func (p *Platform) Members() []grouper.Member {
	return nil
}

func (p *Platform) PostRun() {
}

func (p *Platform) Cleanup() {
}

func (p *Platform) generateRelayServerTOML(relay *Relay) {
	err := os.MkdirAll(p.RelayServerDir(relay), 0755)
	Expect(err).NotTo(HaveOccurred())

	relayServerFile, err := os.Create(p.RelayServerConfigPath(relay))
	Expect(err).NotTo(HaveOccurred())
	defer relayServerFile.Close()

	var relays []*Relay
	for _, r := range p.Topology.Relays {
		if r != relay {
			relays = append(relays, r)
		}
	}

	t, err := template.New("relay_server").Funcs(template.FuncMap{
		"Name":     func() string { return relay.Name },
		"Port":     func() uint16 { return relay.Port },
		"Hostname": func() string { return relay.Hostname },
		"Networks": func() []*Network { return relay.Networks },
		"Drivers":  func() []*Driver { return relay.Drivers },
		"Relays":   func() []*Relay { return relays },
	}).Parse(RelayServerTOML)
	Expect(err).NotTo(HaveOccurred())

	err = t.Execute(io.MultiWriter(relayServerFile), p)
	Expect(err).NotTo(HaveOccurred())
}

func (p *Platform) RelayServerDir(relay *Relay) string {
	return filepath.Join(
		p.Context.RootDir(),
		"weaver",
		"relay",
		"server",
		relay.Name,
	)
}

func (p *Platform) RelayServerConfigPath(relay *Relay) string {
	return filepath.Join(
		p.Context.RootDir(),
		"weaver",
		"relay",
		"server",
		relay.Name,
		"server.toml",
	)
}

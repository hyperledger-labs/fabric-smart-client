/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package integration

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	network2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/generic"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Client interface {
	CallView(fid string, in []byte) (interface{}, error)
	IsTxFinal(txid string) error
}

type BuildServer interface {
	Shutdown()
}

type Builder interface {
	Build(path string) string
}

type Network struct {
	testDir      string
	registry     *registry.Registry
	network      *nwo.Network
	buildServer  BuildServer
	deleteOnStop bool
}

func GenNetwork(startPort int, topologies ...nwo.Topology) (*Network, error) {
	return GenNetworkAt(startPort, "", topologies...)
}

func GenNetworkAt(startPort int, path string, topologies ...nwo.Topology) (*Network, error) {
	RegisterFailHandler(failMe)
	defer ginkgo.GinkgoRecover()

	// Setup the network
	var testDir string
	var err error
	if len(path) != 0 {
		testDir, err = filepath.Abs(path)
		Expect(err).NotTo(HaveOccurred())
		if _, err := os.Stat(testDir); os.IsNotExist(err) {
			err = os.MkdirAll(testDir, 0700)
			Expect(err).NotTo(HaveOccurred())
		}
	} else {
		testDir, err = ioutil.TempDir("", "integration")
		Expect(err).NotTo(HaveOccurred())
	}

	reg := registry.NewRegistry(topologies...)
	reg.NetworkID = common.UniqueName()
	reg.PortCounter = uint16(startPort)
	reg.RootDir = testDir

	var builder Builder
	var buildServer BuildServer
	var platforms []nwo.Platform
	for _, topology := range topologies {
		switch strings.ToLower(topology.Name()) {
		case "fabric":
			bd := network2.NewBuildServer()
			bd.Serve()

			buildServer = bd
			builder = bd.Components()
			reg.Builder = builder

			platform := fabric.NewPlatform(reg, bd.Components())
			reg.AddPlatform(platform.Name(), platform)

			platforms = append(platforms, platform)
		case "generic":
			bd := generic.NewBuildServer()
			bd.Serve()

			buildServer = bd
			builder = bd.Components()
			reg.Builder = builder

			platform := generic.NewPlatform(reg, bd.Components())
			reg.AddPlatform(platform.Name(), platform)

			platforms = append(platforms, platform)
		}
	}
	if len(platforms) == 0 {
		//  Add generic by default if no other platform has been added
		bd := generic.NewBuildServer()
		bd.Serve()

		buildServer = bd
		builder = bd.Components()
		reg.Builder = builder

		platform := generic.NewPlatform(reg, bd.Components())
		reg.AddPlatform(platform.Name(), platform)

		platforms = append(platforms, platform)
	}

	platform := fsc.NewPlatform(reg, builder)
	reg.AddPlatform(platform.Name(), platform)
	platforms = append(platforms, platform)

	n := &Network{
		testDir:      testDir,
		registry:     reg,
		network:      nwo.New(platforms...),
		buildServer:  buildServer,
		deleteOnStop: true,
	}
	n.Generate()
	n.Load()

	return n, nil
}

func LoadNetwork(dir string, topologies ...nwo.Topology) (*Network, error) {
	RegisterFailHandler(failMe)
	defer ginkgo.GinkgoRecover()

	// Setup the network
	buildServer := network2.NewBuildServer()
	buildServer.Serve()

	testDir, err := filepath.Abs(dir)
	Expect(err).NotTo(HaveOccurred())
	_, err = os.Stat(testDir)
	Expect(os.IsNotExist(err)).To(BeEquivalentTo(false))

	reg := registry.NewRegistry(topologies...)
	reg.NetworkID = common.UniqueName()
	reg.RootDir = testDir

	var platforms []nwo.Platform
	for _, topology := range topologies {
		switch strings.ToLower(topology.Name()) {
		case "fabric":
			platforms = append(platforms, fabric.NewPlatform(reg, buildServer.Components()))
		case "generic":
			platforms = append(platforms, generic.NewPlatform(reg, buildServer.Components()))
		}
	}
	if len(platforms) == 0 {
		//  Add generic by default if no other platform has been added
		platforms = append(platforms, generic.NewPlatform(reg, buildServer.Components()))
	}
	platforms = append(platforms, fsc.NewPlatform(reg, buildServer.Components()))

	n := &Network{
		testDir:      testDir,
		registry:     reg,
		network:      nwo.New(platforms...),
		buildServer:  buildServer,
		deleteOnStop: false,
	}
	n.Load()

	return n, nil
}

func (f *Network) Generate() {
	f.network.Generate()
}

func (f *Network) Load() {
	f.network.Load()
}

func (f *Network) Start() {
	f.network.Start()
}

func (f *Network) Stop() {
	defer f.buildServer.Shutdown()
	if f.deleteOnStop {
		defer os.RemoveAll(f.testDir)
	}

	f.network.Stop()
}

func (f *Network) Client(name string) Client {
	return f.registry.ViewClients[name]
}

func (f *Network) Identity(name string) view.Identity {
	id, ok := f.registry.ViewIdentities[name]
	if !ok {
		return []byte(name)
	}
	return id
}

func (f *Network) StopViewNode(id string) {
	f.network.StopViewNode(id)
}

func (f *Network) StartViewNode(id string) {
	f.network.StartViewNode(id)
}

func failMe(message string, callerSkip ...int) {
	panic(message)
}

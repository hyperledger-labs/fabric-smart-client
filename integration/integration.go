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
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Infrastructure struct {
	testDir           string
	ctx               *context.Context
	nwo               *nwo.NWO
	buildServer       *common.BuildServer
	deleteOnStop      bool
	platformFactories map[string]api.PlatformFactory
	topologies        []api.Topology
}

func New(startPort int, path string, topologies ...api.Topology) (*Infrastructure, error) {
	RegisterFailHandler(failMe)
	defer ginkgo.GinkgoRecover()

	var testDir string
	var err error
	if len(path) != 0 {
		testDir, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(testDir); os.IsNotExist(err) {
			err = os.MkdirAll(testDir, 0700)
			if err != nil {
				return nil, err
			}
		}
	} else {
		testDir, err = ioutil.TempDir("", "integration")
		if err != nil {
			return nil, err
		}
	}

	buildServer := common.NewBuildServer()
	buildServer.Serve()
	var builder api.Builder
	builder = buildServer.Client()

	n := &Infrastructure{
		testDir:      testDir,
		ctx:          context.New(testDir, uint16(startPort), builder, topologies...),
		buildServer:  buildServer,
		deleteOnStop: true,
		topologies:   topologies,
		platformFactories: map[string]api.PlatformFactory{
			"fabric": fabric.NewPlatformFactory(),
		},
	}
	return n, nil
}

func Generate(startPort int, topologies ...api.Topology) (*Infrastructure, error) {
	return GenerateAt(startPort, "", topologies...)
}

func GenerateAt(startPort int, path string, topologies ...api.Topology) (*Infrastructure, error) {
	n, err := New(startPort, path, topologies...)
	if err != nil {
		return nil, err
	}
	n.Generate()
	n.Load()

	return n, nil
}

func Load(dir string, topologies ...api.Topology) (*Infrastructure, error) {
	n, err := New(0, dir, topologies...)
	if err != nil {
		return nil, err
	}
	n.deleteOnStop = false
	n.Load()

	return n, nil
}

func (i *Infrastructure) RegisterPlatformFactory(factory api.PlatformFactory) {
	i.platformFactories[factory.Name()] = factory
}

func (i *Infrastructure) Generate() {
	i.initNWO()
	i.nwo.Generate()
}

func (i *Infrastructure) Load() {
	i.initNWO()
	i.nwo.Load()
}

func (i *Infrastructure) Start() {
	if i.nwo == nil {
		panic("call generate or load first")
	}
	i.nwo.Start()
}

func (i *Infrastructure) Stop() {
	if i.nwo == nil {
		panic("call generate or load first")
	}
	defer i.buildServer.Shutdown()
	if i.deleteOnStop {
		defer os.RemoveAll(i.testDir)
	}
	i.nwo.Stop()
}

func (i *Infrastructure) Client(name string) api.ViewClient {
	if i.nwo == nil {
		panic("call generate or load first")
	}

	return i.ctx.ViewClients[name]
}

func (i *Infrastructure) Admin(name string) api.ViewClient {
	if i.nwo == nil {
		panic("call generate or load first")
	}

	return i.ctx.ViewClients[name+".admin"]
}

func (i *Infrastructure) Identity(name string) view.Identity {
	if i.nwo == nil {
		panic("call generate or load first")
	}

	id, ok := i.ctx.ViewIdentities[name]
	if !ok {
		return []byte(name)
	}
	return id
}

func (i *Infrastructure) StopFSCNode(id string) {
	if i.nwo == nil {
		panic("call generate or load first")
	}

	i.nwo.StopFSCNode(id)
}

func (i *Infrastructure) StartFSCNode(id string) {
	if i.nwo == nil {
		panic("call generate or load first")
	}

	i.nwo.StartFSCNode(id)
}

func (i *Infrastructure) initNWO() {
	if i.nwo != nil {
		// skip
		return
	}
	var platforms []api.Platform
	var fscTopology api.Topology
	for _, topology := range i.topologies {
		label := strings.ToLower(topology.Type())
		switch label {
		case "fsc":
			// treat fsc as special
			fscTopology = topology
			continue
		default:
			factory, ok := i.platformFactories[label]
			Expect(ok).To(BeTrue(), "expected to find platform [%s]", label)
			platforms = append(platforms, factory.New(i.ctx, topology, i.buildServer.Client()))
		}
	}
	platforms = append(platforms, fsc.NewPlatform(i.ctx, fscTopology, i.buildServer.Client()))
	for _, platform := range platforms {
		i.ctx.AddPlatform(platform)
	}
	i.nwo = nwo.New(platforms...)
}

func failMe(message string, callerSkip ...int) {
	panic(message)
}

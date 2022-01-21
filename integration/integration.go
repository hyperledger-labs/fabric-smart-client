/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/commands"
	smartclient "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Infrastructure struct {
	TestDir           string
	Ctx               *context.Context
	Nwo               *nwo.NWO
	BuildServer       *common.BuildServer
	DeleteOnStop      bool
	DeleteOnStart     bool
	PlatformFactories map[string]api.PlatformFactory
	Topologies        []api.Topology
	FscPlatform       *fsc.Platform
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
		TestDir:      testDir,
		Ctx:          context.New(testDir, uint16(startPort), builder, topologies...),
		BuildServer:  buildServer,
		DeleteOnStop: true,
		Topologies:   topologies,
		PlatformFactories: map[string]api.PlatformFactory{
			"fabric": fabric.NewPlatformFactory(),
			"weaver": weaver.NewPlatformFactory(),
		},
	}
	return n, nil
}

func Generate(startPort int, race bool, topologies ...api.Topology) (*Infrastructure, error) {
	return GenerateAt(startPort, "", race, topologies...)
}

func GenerateAt(startPort int, path string, race bool, topologies ...api.Topology) (*Infrastructure, error) {
	n, err := New(startPort, path, topologies...)
	if err != nil {
		return nil, err
	}
	if race {
		n.EnableRaceDetector()
	}
	n.Generate()
	n.Load()

	return n, nil
}

func Load(dir string, race bool, topologies ...api.Topology) (*Infrastructure, error) {
	n, err := New(0, dir, topologies...)
	if err != nil {
		return nil, err
	}
	if race {
		n.EnableRaceDetector()
	}
	n.DeleteOnStop = false
	n.Load()

	return n, nil
}

// Clients instantiate a new test integration infrastructure to access view client and CLI
func Clients(dir string, topologies ...api.Topology) (*Infrastructure, error) {
	n, err := New(0, dir, topologies...)
	if err != nil {
		return nil, err
	}
	n.DeleteOnStop = false
	n.InitClients()

	return n, nil
}

func (i *Infrastructure) ViewCmd(node *smartclient.Peer) commands.View {
	p := i.Ctx.PlatformsByName[fsc.TopologyName].(*fsc.Platform)

	return commands.View{
		TLSCA:         path.Join(p.NodeLocalTLSDir(node), "ca.crt"),
		UserCert:      p.LocalMSPIdentityCert(node),
		UserKey:       p.LocalMSPPrivateKey(node),
		NetworkPrefix: p.NetworkID,
		Server:        p.Context.ConnectionConfig(node.Name).Address,
	}
}

func (i *Infrastructure) RegisterPlatformFactory(factory api.PlatformFactory) {
	i.PlatformFactories[factory.Name()] = factory
}

func (i *Infrastructure) Generate() {
	if i.DeleteOnStart {
		if err := os.RemoveAll(i.TestDir); err != nil {
			panic(err)
		}
	}
	i.initNWO()
	i.Nwo.Generate()
}

func (i *Infrastructure) Load() {
	i.initNWO()
	i.Nwo.Load()
}

func (i *Infrastructure) InitClients() {
	i.initNWO()
	i.FscPlatform.InitClients()
}

func (i *Infrastructure) Start() {
	if i.Nwo == nil {
		panic("call generate or load first")
	}
	i.Nwo.Start()
}

func (i *Infrastructure) Stop() {
	if i.Nwo == nil {
		panic("call generate or load first")
	}
	defer i.BuildServer.Shutdown()
	if i.DeleteOnStop {
		defer os.RemoveAll(i.TestDir)
	}
	i.Nwo.Stop()
}

func (i *Infrastructure) Client(name string) api.ViewClient {
	if i.Nwo == nil {
		panic("call generate or load first")
	}

	return i.Ctx.ViewClients[name]
}

func (i *Infrastructure) CLI(name string) api.ViewClient {
	if i.Nwo == nil {
		panic("call generate or load first")
	}

	return i.Ctx.ViewCLIs[name]
}

func (i *Infrastructure) Admin(name string) api.ViewClient {
	if i.Nwo == nil {
		panic("call generate or load first")
	}

	return i.Ctx.ViewClients[name+".admin"]
}

func (i *Infrastructure) Identity(name string) view.Identity {
	if i.Nwo == nil {
		panic("call generate or load first")
	}

	id, ok := i.Ctx.ViewIdentities[name]
	if !ok {
		return []byte(name)
	}
	return id
}

func (i *Infrastructure) StopFSCNode(id string) {
	if i.Nwo == nil {
		panic("call generate or load first")
	}

	i.Nwo.StopFSCNode(id)
}

func (i *Infrastructure) StartFSCNode(id string) {
	if i.Nwo == nil {
		panic("call generate or load first")
	}

	i.Nwo.StartFSCNode(id)
}

func (i *Infrastructure) EnableRaceDetector() {
	i.BuildServer.EnableRaceDetector()
}

func (i *Infrastructure) initNWO() {
	if i.Nwo != nil {
		// skip
		return
	}
	var platforms []api.Platform
	var fscTopology api.Topology
	for _, topology := range i.Topologies {
		label := strings.ToLower(topology.Type())
		switch label {
		case "fsc":
			// treat fsc as special
			fscTopology = topology
			continue
		default:
			factory, ok := i.PlatformFactories[label]
			Expect(ok).To(BeTrue(), "expected to find platform [%s]", label)
			platforms = append(platforms, factory.New(i.Ctx, topology, i.BuildServer.Client()))
		}
	}
	// Add FSC platform
	fcsPlatform := fsc.NewPlatform(i.Ctx, fscTopology, i.BuildServer.Client())
	platforms = append(platforms, fcsPlatform)

	// Register platforms to context
	for _, platform := range platforms {
		i.Ctx.AddPlatform(platform)
	}
	i.Nwo = nwo.New(platforms...)
	i.FscPlatform = fcsPlatform
}

func failMe(message string, callerSkip ...int) {
	panic(message)
}

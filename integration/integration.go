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
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/registry"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ViewClient interface {
	CallView(fid string, in []byte) (interface{}, error)
	IsTxFinal(txid string) error
}

type Builder interface {
	Build(path string) string
}

type Platform interface {
	Name() string

	GenerateConfigTree()
	GenerateArtifacts()
	Load()

	Members() []grouper.Member
	PostRun()
	Cleanup()
}

type PlatformFactory interface {
	Name() string
	New(registry *registry2.Registry, builder Builder) Platform
}

type Infrastructure struct {
	testDir           string
	registry          *registry2.Registry
	nwo               *nwo.NWO
	buildServer       *common.BuildServer
	deleteOnStop      bool
	platformFactories map[string]PlatformFactory
	topologies        []nwo.Topology
}

func New(startPort int, path string, topologies ...nwo.Topology) (*Infrastructure, error) {
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

	reg := registry2.NewRegistry(topologies...)
	reg.NetworkID = common.UniqueName()
	reg.PortCounter = uint16(startPort)
	reg.RootDir = testDir
	var builder Builder

	buildServer := common.NewBuildServer()
	buildServer.Serve()

	builder = buildServer.Client()
	reg.Builder = builder

	n := &Infrastructure{
		testDir:           testDir,
		registry:          reg,
		buildServer:       buildServer,
		deleteOnStop:      true,
		topologies:        topologies,
		platformFactories: map[string]PlatformFactory{},
	}
	return n, nil
}

func Generate(startPort int, topologies ...nwo.Topology) (*Infrastructure, error) {
	return GenerateAt(startPort, "", topologies...)
}

func GenerateAt(startPort int, path string, topologies ...nwo.Topology) (*Infrastructure, error) {
	n, err := New(startPort, path, topologies...)
	if err != nil {
		return nil, err
	}
	n.Generate()
	n.Load()

	return n, nil
}

func Load(dir string, topologies ...nwo.Topology) (*Infrastructure, error) {
	n, err := New(0, dir, topologies...)
	if err != nil {
		return nil, err
	}
	n.deleteOnStop = false
	n.Load()

	return n, nil
}

func (i *Infrastructure) RegisterPlatformFactory(factory PlatformFactory) {
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

func (i *Infrastructure) Client(name string) ViewClient {
	if i.nwo == nil {
		panic("call generate or load first")
	}

	return i.registry.ViewClients[name]
}

func (i *Infrastructure) Identity(name string) view.Identity {
	if i.nwo == nil {
		panic("call generate or load first")
	}

	id, ok := i.registry.ViewIdentities[name]
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
	var platforms []nwo.Platform
	for _, topology := range i.topologies {
		label := strings.ToLower(topology.Name())
		switch label {
		case "fabric":
			platforms = append(platforms, fabric.NewPlatform(i.registry, i.buildServer.Client()))
		case "generic":
			platforms = append(platforms, generic.NewPlatform(i.registry, i.buildServer.Client()))
		case "fsc":
			// skip
			continue
		default:
			factory, ok := i.platformFactories[label]
			Expect(ok).To(BeTrue(), "expected to find platform [%s]", label)
			platforms = append(platforms, factory.New(i.registry, i.buildServer.Client()))
		}
	}
	if len(platforms) == 0 {
		//  Add generic by default if no other platform has been added
		platforms = append(platforms, generic.NewPlatform(i.registry, i.buildServer.Client()))
	}
	platforms = append(platforms, fsc.NewPlatform(i.registry, i.buildServer.Client()))
	i.nwo = nwo.New(platforms...)
}

func failMe(message string, callerSkip ...int) {
	panic(message)
}

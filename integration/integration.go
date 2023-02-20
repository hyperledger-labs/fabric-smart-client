/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/commands"
	smartclient "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/orion"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var logger = flogging.MustGetLogger("fsc.integration")

type Configuration struct {
	StartPort int
}

type Infrastructure struct {
	TestDir           string
	StartPort         int
	Ctx               *context.Context
	NWO               *nwo.NWO
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
	builder := buildServer.Client()

	n := &Infrastructure{
		TestDir:      testDir,
		StartPort:    startPort,
		Ctx:          context.New(testDir, uint16(startPort), builder, topologies...),
		BuildServer:  buildServer,
		DeleteOnStop: true,
		Topologies:   topologies,
		PlatformFactories: map[string]api.PlatformFactory{
			"fabric":     fabric.NewPlatformFactory(),
			"weaver":     weaver.NewPlatformFactory(),
			"monitoring": monitoring.NewPlatformFactory(),
			"orion":      orion.NewPlatformFactory(),
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

	return n, nil
}

func Load(startPort int, dir string, race bool, topologies ...api.Topology) (*Infrastructure, error) {
	n, err := New(startPort, dir, topologies...)
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
		logger.Infof("Delete test folder [%s]", i.TestDir)
		if err := os.RemoveAll(i.TestDir); err != nil {
			panic(err)
		}
	}
	i.initNWO()
	i.NWO.Generate()
	i.storeAdditionalConfigurations()
}

func (i *Infrastructure) Load() {
	i.initNWO()
	i.NWO.Load()
}

func (i *Infrastructure) InitClients() {
	i.initNWO()
	i.FscPlatform.InitClients()
}

func (i *Infrastructure) Start() {
	if i.NWO == nil {
		panic("call generate or load first")
	}
	i.NWO.Start()
}

func (i *Infrastructure) Stop() {
	if i.NWO == nil {
		panic("call generate or load first")
	}
	defer i.BuildServer.Shutdown()
	if i.DeleteOnStop {
		defer os.RemoveAll(i.TestDir)
	}
	i.NWO.Stop()
}

func (i *Infrastructure) Serve() error {
	serve := make(chan error, 10)
	go handleSignals(map[os.Signal]func(){
		syscall.SIGINT: func() {
			logger.Infof("Received SIGINT, exiting...")
			serve <- nil
		},
		syscall.SIGTERM: func() {
			logger.Infof("Received SIGTERM, exiting...")
			serve <- nil
		},
		syscall.SIGSTOP: func() {
			logger.Infof("Received SIGSTOP, exiting...")
			serve <- nil
		},
		syscall.SIGHUP: func() {
			logger.Infof("Received SIGHUP, exiting...")
			serve <- nil
		},
	})
	logger.Infof("All GOOD, networks up and running...")
	logger.Infof("If you want to shut down the networks, press CTRL+C")
	logger.Infof("Open another terminal to interact with the networks")
	return <-serve
}

func (i *Infrastructure) Client(name string) api.ViewClient {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	return i.Ctx.ViewClients[name]
}

func (i *Infrastructure) CLI(name string) api.ViewClient {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	return i.Ctx.ViewCLIs[name]
}

func (i *Infrastructure) Identity(name string) view.Identity {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	id, ok := i.Ctx.ViewIdentities[name]
	if !ok {
		return []byte(name)
	}
	return id
}

func (i *Infrastructure) StopFSCNode(id string) {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	i.NWO.StopFSCNode(id)
}

func (i *Infrastructure) StartFSCNode(id string) {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	i.NWO.StartFSCNode(id)
}

func (i *Infrastructure) EnableRaceDetector() {
	i.BuildServer.EnableRaceDetector()
}

func (i *Infrastructure) initNWO() {
	if i.NWO != nil {
		// skip
		return
	}
	var platforms []api.Platform
	var fscTopology api.Topology
	if _, ok := i.PlatformFactories["fsc"]; !ok {
		i.RegisterPlatformFactory(&fscDefaultPlatformFactory{})
	}
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
	fcsPlatform := i.PlatformFactories["fsc"].New(i.Ctx, fscTopology, i.BuildServer.Client()).(*fsc.Platform)
	platforms = append(platforms, fcsPlatform)

	// Register platforms to context
	for _, platform := range platforms {
		i.Ctx.AddPlatform(platform)
	}
	i.NWO = nwo.New(i.Ctx, platforms...)
	i.FscPlatform = fcsPlatform
}

func (i *Infrastructure) storeAdditionalConfigurations() {
	// store configuration
	conf := &Configuration{
		StartPort: i.StartPort,
	}
	raw, err := json.Marshal(conf)
	if err != nil {
		panic(err)
	}
	if err := ioutil.WriteFile(filepath.Join(i.TestDir, "conf.json"), raw, 0770); err != nil {
		panic(err)
	}

	// store topology
	t := api.Topologies{Topologies: i.Topologies}
	raw, err = t.Export()
	if err != nil {
		panic(err)
	}
	if err := ioutil.WriteFile(filepath.Join(i.TestDir, "topology.yaml"), raw, 0770); err != nil {
		panic(err)
	}
}

func failMe(message string, callerSkip ...int) {
	panic(message)
}

func handleSignals(handlers map[os.Signal]func()) {
	var signals []os.Signal
	for sig := range handlers {
		signals = append(signals, sig)
	}
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, signals...)
	for sig := range signalChan {
		handlers[sig]()
	}
}

type fscDefaultPlatformFactory struct{}

func (p *fscDefaultPlatformFactory) Name() string {
	return "fsc"
}
func (p *fscDefaultPlatformFactory) New(registry api.Context, t api.Topology, builder api.Builder) api.Platform {
	return fsc.NewPlatform(registry, t, builder)
}

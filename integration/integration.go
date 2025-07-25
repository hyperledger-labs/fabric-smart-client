/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"cmp"
	"encoding/json"
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
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var logger = logging.MustGetLogger()

func init() {
	c := logging.Config{}
	// set grpc logging level as default to error; note that flogging default log level is just info
	c.LogSpec = cmp.Or(os.Getenv("FABRIC_LOGGING_SPEC"), "grpc=error:info")
	logging.Init(c)
}

type Configuration struct {
	StartPort int
}

const WithRaceDetection = true

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
	gomega.RegisterFailHandler(failMe)
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
		logger.Infof("Instantiating Integration infrastructure at [%s -> %s]", path, testDir)
	} else {
		testDir, err = os.MkdirTemp("", "integration")
		if err != nil {
			return nil, err
		}
	}

	buildServer := common.NewBuildServer("-tags", "pkcs11")
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
			"monitoring": monitoring.NewPlatformFactory(),
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

func (i *Infrastructure) ViewCmd(node *smartclient.Replica) commands.View {
	p := i.Ctx.PlatformsByName[fsc.TopologyName].(*fsc.Platform)

	return commands.View{
		TLSCA:         path.Join(p.NodeLocalTLSDir(node.Peer), "ca.crt"),
		UserCert:      p.LocalMSPIdentityCert(node.Peer),
		UserKey:       p.LocalMSPPrivateKey(node.Peer),
		NetworkPrefix: p.NetworkID,
		Server:        p.Context.ConnectionConfig(node.UniqueName).Address,
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
	logger.Infof("stopping ...")
	if i.NWO == nil {
		panic("call generate or load first")
	}
	defer i.BuildServer.Shutdown(i.DeleteOnStop)
	if i.DeleteOnStop {
		defer utils.IgnoreErrorWithOneArg(os.RemoveAll, i.TestDir)
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

func (i *Infrastructure) Client(name string) api.GRPCClient {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	return i.Ctx.ViewClients[name]
}

func (i *Infrastructure) WebClient(name string) api.WebClient {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	return i.Ctx.WebClients[name]
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

	for _, topology := range i.Topologies {
		label := strings.ToLower(topology.Type())
		switch label {
		case "fsc":
			// treat fsc as last topo
			fscTopology = topology
			continue
		default:
			logger.Infof("Register %s", label)
			factory, ok := i.PlatformFactories[label]
			gomega.Expect(ok).To(gomega.BeTrue(), "expected to find platform [%s]", label)
			platforms = append(platforms, factory.New(i.Ctx, topology, i.BuildServer.Client()))
		}
	}

	// Add FSC platform
	if fscTopology != nil {
		if _, ok := i.PlatformFactories["fsc"]; !ok {
			i.RegisterPlatformFactory(&fscDefaultPlatformFactory{})
		}
		factory := i.PlatformFactories["fsc"]

		fscPlatform := factory.New(i.Ctx, fscTopology, i.BuildServer.Client())
		platforms = append(platforms, fscPlatform)
		i.FscPlatform = fscPlatform.(*fsc.Platform)
	}

	// Register platforms to context
	for _, platform := range platforms {
		i.Ctx.AddPlatform(platform)
	}
	i.NWO = nwo.New(i.Ctx, platforms...)
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
	if err := os.WriteFile(filepath.Join(i.TestDir, "conf.json"), raw, 0770); err != nil {
		panic(err)
	}

	// store topology
	t := api.Topologies{Topologies: i.Topologies}
	raw, err = t.Export()
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(i.TestDir, "topology.yaml"), raw, 0770); err != nil {
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

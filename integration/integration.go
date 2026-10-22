/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	nwo_context "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/commands"
	smartclient "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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
	NWOCtx            *nwo_context.NWOContext
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
			err = os.MkdirAll(testDir, 0o700)
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

	// build parameter: add pkcs11 tag only when the topology contains HSM nodes
	var params []string
outer:
	for _, t := range topologies {
		if ft, ok := t.(*fsc.Topology); ok {
			for _, n := range ft.Nodes {
				fo := fabric.Options(n.Options)
				if fo.DefaultIdentityByHSM() || len(fo.X509IdentitiesByHSM()) > 0 {
					params = append(params, "-tags", "pkcs11")
					break outer
				}
			}
		}
	}

	// build with coverage profiling, more details: https://go.dev/doc/build-cover
	//
	// -coverpkg is required: without it, `-cover` only instruments each spawned
	// binary's own packages, which all live under integration/. Those are then
	// discarded by scripts/filter-coverage.sh (which excludes /integration/), so
	// the integration tests would report no coverage at all. Instrumenting the
	// whole module credits the platform/core code the FSC nodes exercise at
	// runtime, which is the coverage the integration tests are meant to measure.
	//
	// Downstream modules that embed FSC nodes (e.g. fabric-token-sdk) run their
	// own code inside these node binaries and need their packages instrumented
	// instead of — or in addition to — FSC's. Go's standard channel for that is
	// -coverpkg supplied via GOFLAGS. Respect it: only fall back to instrumenting
	// the FSC module when the caller has not set -coverpkg, so a consumer that
	// exports GOFLAGS=-coverpkg=<their-module>/... has it honoured rather than
	// silently overridden, while FSC's own coverage build is unchanged.
	if _, ok := os.LookupEnv("GOCOVERDIR"); ok {
		params = append(params, "-cover")
		if !goflagsHasCoverpkg() {
			params = append(params, "-coverpkg=github.com/hyperledger-labs/fabric-smart-client/...")
		}
	}

	buildServer := common.NewBuildServer(params...)
	buildServer.Serve()
	builder := buildServer.Client()

	n := &Infrastructure{
		TestDir:      testDir,
		StartPort:    startPort,
		NWOCtx:       nwo_context.New(testDir, uint16(startPort), builder, topologies...),
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

// goflagsHasCoverpkg reports whether the caller has already supplied a -coverpkg
// flag through the GOFLAGS environment variable. GOFLAGS is a space-separated
// list of flags that every go command merges into its own; an explicit -coverpkg
// there is the standard way for a consumer to select the packages instrumented
// in the cover-built node binaries.
func goflagsHasCoverpkg() bool {
	for f := range strings.FieldsSeq(os.Getenv("GOFLAGS")) {
		if strings.HasPrefix(f, "-coverpkg") || strings.HasPrefix(f, "--coverpkg") {
			return true
		}
	}
	return false
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

// ViewCmd builds the connection parameters the CLI needs to invoke a view on
// node, from the registered fsc platform. It fails the test if no fsc
// platform is registered.
func (i *Infrastructure) ViewCmd(node *smartclient.Replica) commands.View {
	platform := i.NWOCtx.PlatformByName(fsc.TopologyName)
	gomega.Expect(platform).NotTo(gomega.BeNil(), "expected an fsc platform named [%s]", fsc.TopologyName)
	p, ok := platform.(*fsc.Platform)
	gomega.Expect(ok).To(gomega.BeTrue(), "expected the fsc platform to be a *fsc.Platform")

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
		defer func() { _ = os.RemoveAll(i.TestDir) }()
	}
	i.NWO.Stop()
}

func (*Infrastructure) Serve() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Infof("All GOOD, networks up and running...")
	logger.Infof("If you want to shut down the networks, press CTRL+C")
	logger.Infof("Open another terminal to interact with the networks")

	<-ctx.Done()
	logger.Infof("Received signal, exiting...")
	return nil
}

func (i *Infrastructure) Client(name string) api.RemoteViewClient {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	return i.NWOCtx.ViewClients[name]
}

func (i *Infrastructure) WebClient(name string) api.WebClient {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	return i.NWOCtx.WebClients[name]
}

func (i *Infrastructure) CLI(name string) api.ViewClient {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	return i.NWOCtx.ViewCLIs[name]
}

func (i *Infrastructure) Identity(name string) view.Identity {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	id, ok := i.NWOCtx.ViewIdentities[name]
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

// KillFSCNode sends SIGKILL to the FSC node with the given id and returns once
// it is gone. Restart it with StartFSCNode.
func (i *Infrastructure) KillFSCNode(id string) {
	if i.NWO == nil {
		panic("call generate or load first")
	}

	i.NWO.KillFSCNode(id)
}

// StopPeer stops the Fabric peer with the given name and returns once it is gone.
// The test fails unless exactly one running Fabric network has a peer of that name.
func (i *Infrastructure) StopPeer(name string) { i.NWO.StopMember(i.fabricMember("peer", name)) }

// StartPeer restarts the Fabric peer stopped by StopPeer and returns once it is ready.
func (i *Infrastructure) StartPeer(name string) { i.NWO.StartMember(i.fabricMember("peer", name)) }

// StopOrderer stops the Fabric orderer with the given name and returns once it is gone.
// The test fails unless exactly one running Fabric network has an orderer of that name.
func (i *Infrastructure) StopOrderer(name string) { i.NWO.StopMember(i.fabricMember("orderer", name)) }

// StartOrderer restarts the Fabric orderer stopped by StopOrderer and returns once it is ready.
func (i *Infrastructure) StartOrderer(name string) {
	i.NWO.StartMember(i.fabricMember("orderer", name))
}

// fabricMember returns the nwo member name of the Fabric peer or orderer
// (kind) with the given name; see network.Network.Members for the naming.
func (i *Infrastructure) fabricMember(kind, name string) string {
	if i.NWO == nil {
		panic("call generate or load first")
	}
	suffix := "." + kind + "." + name
	var found []string
	for _, m := range i.NWO.Members {
		if strings.HasSuffix(m.Name, suffix) {
			found = append(found, m.Name)
		}
	}
	gomega.Expect(found).To(gomega.HaveLen(1), "expected exactly one %s named [%s], found %v", kind, name, found)
	return found[0]
}

// EventuallyConnectionState polls the metrics of the FSC node until every
// fsc_connection_state series that conn selects is in state want, and fails the
// test if that does not happen within timeout. An empty field of conn matches
// any value, and at least one series must match. Leaving Network or Channel
// empty therefore requires the matching connections of every network or
// channel to be in want, so one that is down cannot hide behind another.
//
// The series reports what the node last observed, which can be stale: the
// orderer series changes only when the node broadcasts, so after stopping an
// orderer it keeps its last value until a broadcast fails. Make the node act
// on the connection before asserting a state that follows a fault.
func (i *Infrastructure) EventuallyConnectionState(node string, conn fsc.Connection, want connstate.State, timeout time.Duration) {
	client := i.WebClient(node)
	gomega.Expect(client).NotTo(gomega.BeNil(), "no web client for node [%s]", node)
	eventuallyConnectionState(gomega.Default, client.Metrics, conn, want, timeout,
		"node [%s]: connection state of %+v never reached %d", node, conn, want)
}

// eventuallyConnectionState is EventuallyConnectionState with the metrics
// source and the Gomega instance passed in, so it can be tested without a
// network.
func eventuallyConnectionState(g gomega.Gomega, metrics func() (map[string]*dto.MetricFamily, error), conn fsc.Connection, want connstate.State, timeout time.Duration, msg ...any) {
	g.Eventually(func(g gomega.Gomega) {
		families, err := metrics()
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(fsc.ParseConnectionState(families, conn)).To(gomega.HaveEach(want))
	}).WithOffset(2).WithTimeout(timeout).WithPolling(500*time.Millisecond).Should(gomega.Succeed(), msg...)
}

// StopPostgres stops the Postgres container of the database dbName, which nwo
// started for an FSC node, and returns once it is stopped. The container keeps
// its data and port, so StartPostgres brings the same database back.
func (*Infrastructure) StopPostgres(dbName string) {
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(d.StopContainer(dbName)).To(gomega.Succeed())
}

// StartPostgres restarts the container stopped by StopPostgres and returns once
// its healthcheck reports the database ready.
func (*Infrastructure) StartPostgres(dbName string) {
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(d.StartContainer(dbName)).To(gomega.Succeed())
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
			platforms = append(platforms, factory.New(i.NWOCtx, topology, i.BuildServer.Client()))
		}
	}

	// Add FSC platform
	if fscTopology != nil {
		if _, ok := i.PlatformFactories["fsc"]; !ok {
			i.RegisterPlatformFactory(&fscDefaultPlatformFactory{})
		}
		factory := i.PlatformFactories["fsc"]

		fscPlatform := factory.New(i.NWOCtx, fscTopology, i.BuildServer.Client())
		platforms = append(platforms, fscPlatform)
		i.FscPlatform = fscPlatform.(*fsc.Platform)
	}

	// Register platforms to context
	for _, platform := range platforms {
		i.NWOCtx.AddPlatform(platform)
	}
	i.NWO = nwo.New(i.NWOCtx, platforms...)
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
	if err := os.WriteFile(filepath.Join(i.TestDir, "conf.json"), raw, 0o770); err != nil {
		panic(err)
	}

	// store topology
	t := api.Topologies{Topologies: i.Topologies}
	raw, err = t.Export()
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(i.TestDir, "topology.yaml"), raw, 0o770); err != nil {
		panic(err)
	}
}

func failMe(message string, _ ...int) {
	panic(message)
}

type fscDefaultPlatformFactory struct{}

func (*fscDefaultPlatformFactory) Name() string {
	return "fsc"
}

func (*fscDefaultPlatformFactory) New(registry api.Context, t api.Topology, builder api.Builder) api.Platform {
	return fsc.NewPlatform(registry, t, builder)
}

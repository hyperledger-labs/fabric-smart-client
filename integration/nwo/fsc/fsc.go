/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	runner2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/commands"
	node2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/otlp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	tracing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	client3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client/cmd"
	client2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
	"github.com/miracl/conflate"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/spf13/viper"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
)

var logger = logging.MustGetLogger()

func init() {
	// define the unmarshallers for the given file extensions, blank extension is the global unmarshaller
	conflate.Unmarshallers = conflate.UnmarshallerMap{
		".jsn":  {conflate.JSONUnmarshal},
		".yaml": {conflate.YAMLUnmarshal},
		".yml":  {conflate.YAMLUnmarshal},
		".toml": {conflate.TOMLUnmarshal},
		".tml":  {conflate.TOMLUnmarshal},
		"":      {conflate.JSONUnmarshal, conflate.YAMLUnmarshal, conflate.TOMLUnmarshal},
	}
}

const (
	ListenPort api.PortName = "Listen" // Port at which the fsc node might listen for some service
	ViewPort   api.PortName = "View"   // Port at which the View Service Server respond
	P2PPort    api.PortName = "P2P"    // Port at which the P2P Communication Layer respond
	WebPort    api.PortName = "Web"    // Port at which the Web Server respond
)

func WithReplicationFactor(factor int) node2.Option {
	return func(o *node2.Options) error {
		o.Put("Replication", factor)
		return nil
	}
}

func WithAlias(alias string) node2.Option {
	return func(o *node2.Options) error {
		o.AddAlias(alias)
		return nil
	}
}

type Platform struct {
	Context           api.Context
	NetworkID         string
	Builder           *Builder
	Topology          *Topology
	EventuallyTimeout time.Duration

	Organizations            []*node2.Organization
	Peers                    []*node2.Replica
	Resolvers                []*Resolver
	Routing                  map[string][]string
	colorIndex               int
	metricsAggregatorProcess ifrit.Process
	cleanDB                  func()
}

func NewPlatform(Registry api.Context, t api.Topology, builderClient BuilderClient) *Platform {
	p := &Platform{
		Context:           Registry,
		NetworkID:         common.UniqueName(),
		Builder:           &Builder{client: builderClient},
		Topology:          t.(*Topology),
		EventuallyTimeout: 10 * time.Minute,
	}
	p.CheckTopology()
	return p
}

func (p *Platform) Name() string {
	return TopologyName
}

func (p *Platform) Type() string {
	return TopologyName
}

func (p *Platform) GenerateConfigTree() {
	p.GenerateCryptoConfig()
}

func (p *Platform) GenerateArtifacts() {
	sess, err := p.Cryptogen(commands.Generate{
		Config: p.CryptoConfigPath(),
		Output: p.CryptoPath(),
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, p.EventuallyTimeout).Should(gexec.Exit(0))

	p.ConcatenateTLSCACertificates()

	p.GenerateResolverMap()

	p.generatePostgresConfiguration()

	// Generate core.yaml for all fsc nodes by including all the additional configurations coming
	// from other platforms
	for _, peer := range p.Peers {
		address := p.PeerAddress(peer, ListenPort)
		p.setIdentities(address, peer)

		p.GenerateCoreConfig(peer)
		if p.P2PCommunicationType() == WebSocket {
			p.GenerateRoutingConfig()
		}

		c := view2.Config{
			Version: 0,
			Address: p.PeerAddress(peer, ListenPort),
			TLSConfig: view2.TLSConfig{
				PeerCACertPath: path.Join(p.NodeLocalTLSDir(peer.Peer), "ca.crt"),
				Timeout:        10 * time.Minute,
			},
			SignerConfig: view2.SignerConfig{
				IdentityPath: p.LocalMSPIdentityCert(peer.Peer),
				KeyPath:      p.LocalMSPPrivateKey(peer.Peer),
			},
		}
		gomega.Expect(c.ToFile(p.NodeClientConfigPath(peer))).ToNot(gomega.HaveOccurred())
	}

	// Generate commands
	for _, node := range p.Peers {
		if len(node.ExecutablePath) == 0 {
			p.GenerateCmd(nil, node)
		}
	}
}

// generatePostgresConfiguration allocates network ports for the postgres databases spawned by NWO.
// It finds all postgres databases via their unique db names and assigns a free network port to it,
// then it updates each fsc node configuration with the updated port details.
func (p *Platform) generatePostgresConfiguration() {
	dbPorts := make(map[string]string)

	// search all unique postgres instances available in our topology
	for _, peer := range p.Peers {
		for n, pp := range peer.Options.GetPostgresPersistences() {
			cfg, err := postgres2.ConfigFromDataSource(pp.DataSource)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// if we have seen this db already, we update the node's configuration; otherwise reserve a free port
			port, ok := dbPorts[cfg.DBName]
			if !ok {
				port = strconv.Itoa(int(p.Context.ReservePort()))
				dbPorts[cfg.DBName] = port
			}

			// update the port in the node options,
			// so we store it below in the core.yaml
			cfg.Port = port
			pp.DataSource = cfg.DataSource()
			peer.Options.PutPostgresPersistence(n, *pp)
		}
	}
}

func (p *Platform) setIdentities(address string, peer *node2.Replica) {
	cc := &grpc.ConnectionConfig{
		Address:           address,
		TLSEnabled:        true,
		TLSRootCertFile:   path.Join(p.NodeLocalTLSDir(peer.Peer), "ca.crt"),
		ConnectionTimeout: 10 * time.Minute,
	}
	p.Context.SetConnectionConfig(peer.UniqueName, cc)

	clientID, err := p.GetSigningIdentity(peer.Peer)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	p.Context.SetClientSigningIdentity(peer.Name, clientID)

	adminID, err := p.GetAdminSigningIdentity(peer.Peer)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	p.Context.SetAdminSigningIdentity(peer.Name, adminID)

	cert, err := os.ReadFile(p.LocalMSPIdentityCert(peer.Peer))
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	p.Context.SetViewIdentity(peer.Name, cert)
}

func (p *Platform) Load() {
}

func (p *Platform) members(bootstrap bool) []grouper.Member {
	members := grouper.Members{}
	for _, node := range p.Peers {
		if node.Bootstrap == bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.FSCNodeRunner(node)})
		}
	}
	return members
}

func (p *Platform) Members() []grouper.Member {
	return append(p.members(true), p.members(false)...)
}

func (p *Platform) PreRun() {
	startPostgres(p)
}

// startPostgres spawns a Postgres container for each database as specified via the platform topology.
// It sets Platform.cleanDB function to allow shutdown of all spawned databases. If an error occurs, we panic
func startPostgres(p *Platform) {
	// find all postgres databases we need to start via node configurations
	configs := make(map[string]*postgres2.ContainerConfig)
	for _, node := range p.Peers {
		for _, sqlOpts := range node.Options.GetPostgresPersistences() {
			if _, ok := configs[sqlOpts.DataSource]; !ok {
				c, err := postgres2.ConfigFromDataSource(sqlOpts.DataSource)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
				configs[sqlOpts.DataSource] = c
			}
		}
	}

	logger.Infof("Starting DBs for following data sources: [%s]...", logging.Keys(configs))

	closeFuncs := make([]func(), 0, len(configs))

	// start all postgres instances
	for _, c := range configs {
		// get a logger for our postgres db
		l := logging.MustGetLogger(c.DBName)

		// we ignore the returned connection str
		closeFunc, _, err := postgres2.StartPostgres(context.TODO(), c, l)
		if err != nil {
			// close all started instances
			for _, f := range closeFuncs {
				if f != nil {
					f()
				}
			}
			// we cannot start the database, thus no way to move on, time for panic! :P
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		}

		closeFuncs = append(closeFuncs, closeFunc)
	}

	// merge all close functions
	p.cleanDB = func() {
		for _, f := range closeFuncs {
			if f != nil {
				f()
			}
		}
	}
}

func (p *Platform) PostRun(bool) {
	for _, peer := range p.Peers {
		v := p.viper(peer)

		address := v.GetString("fsc.grpc.address")
		p.setIdentities(address, peer)
	}
	tracerProvider, err := tracing2.NewProviderFromConfig(tracing2.Config{
		Provider: p.Topology.Monitoring.TracingType,
		File: tracing2.FileConfig{
			Path: "./client-trace.out",
		},
		Otlp: tracing2.OtlpConfig{
			Address: fmt.Sprintf("0.0.0.0:%d", otlp.JaegerCollectorPort),
		},
	})
	if err != nil {
		panic(err)
	}

	for _, node := range p.Peers {

		v := p.viper(node)

		// Prepare GRPC Client, Web Client, and CLI

		// GRPC client
		grpcClient, err := client3.NewClient(
			&client3.Config{
				ID:               v.GetString("fsc.id"),
				ConnectionConfig: p.Context.ConnectionConfig(node.UniqueName),
			},
			p.Context.ClientSigningIdentity(node.Name),
			tracerProvider,
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		p.Context.SetViewClient(node.Name, grpcClient)
		p.Context.SetViewClient(node.ID(), grpcClient)
		for _, identity := range p.Context.GetViewIdentityAliases(node.ID()) {
			p.Context.SetViewClient(identity, grpcClient)
		}
		for _, identity := range p.Context.GetViewIdentityAliases(node.Name) {
			p.Context.SetViewClient(identity, grpcClient)
		}
		for _, alias := range node.Aliases {
			p.Context.SetViewClient(alias, grpcClient)
		}

		// Web Client
		webClientConfig, err := client.NewWebClientConfigFromFSC(p.NodeDir(node))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		webClient, err := client2.NewClient(webClientConfig)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		p.Context.SetWebClient(node.Name, webClient)
		p.Context.SetWebClient(node.ID(), webClient)
		for _, identity := range p.Context.GetViewIdentityAliases(node.ID()) {
			p.Context.SetWebClient(identity, webClient)
		}
		for _, identity := range p.Context.GetViewIdentityAliases(node.Name) {
			p.Context.SetWebClient(identity, webClient)
		}
		for _, alias := range node.Aliases {
			p.Context.SetWebClient(alias, webClient)
		}

		// CLI
		cli := &fscCLIViewClient{
			timeout: p.EventuallyTimeout,
			p:       p,
			CMD: commands.View{
				TLSCA:         path.Join(p.NodeLocalTLSDir(node.Peer), "ca.crt"),
				UserCert:      p.LocalMSPIdentityCert(node.Peer),
				UserKey:       p.LocalMSPPrivateKey(node.Peer),
				NetworkPrefix: p.NetworkID,
				Server:        p.Context.ConnectionConfig(node.UniqueName).Address,
			},
		}
		p.Context.SetCLI(node.Name, cli)
		p.Context.SetCLI(node.ID(), cli)
	}
}

func (p *Platform) viper(peer *node2.Replica) *viper.Viper {
	v := viper.New()
	v.SetConfigFile(p.NodeConfigPath(peer))
	err := v.ReadInConfig() // Find and read the config file
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return v
}

func (p *Platform) Cleanup() {
	if p.metricsAggregatorProcess != nil {
		p.metricsAggregatorProcess.Signal(os.Kill)
	}

	if p.cleanDB != nil {
		p.cleanDB()
	}
}

func (p *Platform) CheckTopology() {
	orgName := "fsc"

	org := &node2.Organization{
		ID:            orgName,
		Name:          orgName,
		MSPID:         orgName + "MSP",
		Domain:        strings.ToLower(orgName) + ".example.com",
		EnableNodeOUs: false,
		Users:         2,
	}
	p.Organizations = append(p.Organizations, org)
	users := map[string]int{}
	userNames := map[string][]string{}
	bootstrapNodeFound := false

	if len(p.Topology.Nodes) == 0 {
		return
	}

	for _, node := range p.Topology.Nodes {
		if p.Topology.P2PCommunicationType != WebSocket && node.Options.ReplicationFactor() > 1 {
			panic("replication only supported for websocket p2p communication")
		}
		for _, uniqueName := range node.ReplicaUniqueNames() {
			var extraIdentities []*node2.PeerIdentity
			peer := node2.NewReplica(&node2.Peer{
				Name:            node.Name,
				Organization:    org.Name,
				Bootstrap:       node.Bootstrap,
				ExecutablePath:  node.ExecutablePath,
				ExtraIdentities: extraIdentities,
				Node:            node,
				Aliases:         node.Options.Aliases(),
			}, uniqueName)
			peer.Admins = []string{
				p.AdminLocalMSPIdentityCert(peer.Peer),
			}
			ports := api.Ports{}
			for _, portName := range PeerPortNames() {
				ports[portName] = p.Context.ReservePort()
			}
			p.Context.SetPortsByPeerID("fsc", peer.ID(), ports)
			p.Context.SetHostByPeerID("fsc", peer.ID(), "127.0.0.1")
			p.Peers = append(p.Peers, peer)
			users[orgName] = users[orgName] + 1
			userNames[orgName] = append(userNames[orgName], node.Name)

			// Is this a bootstrap node/
			if node.Bootstrap {
				bootstrapNodeFound = true
			}
		}
	}

	for _, organization := range p.Organizations {
		organization.Users += users[organization.Name]
		organization.UserNames = append(userNames[organization.Name], "User1", "User2")
	}

	if !bootstrapNodeFound {
		p.Topology.Nodes[0].Bootstrap = true
	}
}

func (p *Platform) OperationAddress(peer *node2.Replica) string {
	fabricHost := "fabric"
	if runtime.GOOS == "darwin" {
		fabricHost = "host.docker.internal"
	}
	return net.JoinHostPort(fabricHost, strconv.Itoa(int(p.Context.PortsByPeerID("fsc", peer.ID())[WebPort])))
}

func (p *Platform) InitClients() {
	p.Load()
	p.PostRun(false)
}

func (p *Platform) FSCCLI(command common.Command) (*gexec.Session, error) {
	cmd := common.NewCommand(p.Builder.FSCCLI(), command)
	return p.StartSession(cmd, command.SessionName())
}

func (p *Platform) Cryptogen(command common.Command) (*gexec.Session, error) {
	cmd := common.NewCommand(p.Builder.FSCCLI(), command)
	return p.StartSession(cmd, command.SessionName())
}

func (p *Platform) StartSession(cmd *exec.Cmd, name string) (*gexec.Session, error) {
	ansiColorCode := p.nextColor()
	_, _ = fmt.Fprintf(
		ginkgo.GinkgoWriter,
		"\x1b[33m[d]\x1b[%s[%s]\x1b[0m starting %s %s\n",
		ansiColorCode,
		name,
		filepath.Base(cmd.Args[0]),
		strings.Join(cmd.Args[1:], " "),
	)
	return gexec.Start(
		cmd,
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", ansiColorCode, name),
			ginkgo.GinkgoWriter,
		),
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", ansiColorCode, name),
			ginkgo.GinkgoWriter,
		),
	)
}

func (p *Platform) CryptoConfigPath() string {
	return filepath.Join(p.Context.RootDir(), "fsc", "crypto-config.yaml")
}

func (p *Platform) P2PCommunicationType() P2PCommunicationType {
	if typ := p.Topology.P2PCommunicationType; len(typ) > 0 {
		return typ
	}
	return LibP2P
}

func (p *Platform) RoutingConfigPath() string {
	return filepath.Join(p.Context.RootDir(), "fsc", "routing-config.yaml")
}

func (p *Platform) GenerateCryptoConfig() {
	gomega.Expect(os.MkdirAll(p.CryptoPath(), 0755)).NotTo(gomega.HaveOccurred())

	crypto, err := os.Create(p.CryptoConfigPath())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer utils.IgnoreErrorFunc(crypto.Close)

	t, err := template.New("crypto").Parse(p.Topology.Templates.CryptoTemplate())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	gomega.Expect(t.Execute(io.MultiWriter(crypto), p)).NotTo(gomega.HaveOccurred())
}

func (p *Platform) GenerateRoutingConfig() {
	routing, err := os.Create(p.RoutingConfigPath())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer utils.IgnoreErrorFunc(routing.Close)

	t, err := template.New("routing").Parse(node2.RoutingTemplate)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	gomega.Expect(t.Execute(io.MultiWriter(routing), p)).NotTo(gomega.HaveOccurred())
}

func (p *Platform) GenerateCoreConfig(peer *node2.Replica) {
	err := os.MkdirAll(p.NodeDir(peer), 0755)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	core, err := os.Create(p.NodeConfigPath(peer))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer utils.IgnoreErrorFunc(core.Close)

	var extensions []string
	for _, extensionsByPeerID := range p.Context.ExtensionsByPeerID(peer.UniqueName) {
		// if len(extensionsByPeerID) > 1, we need a merge
		if len(extensionsByPeerID) > 1 {
			c := conflate.New()
			for _, ext := range extensionsByPeerID {
				gomega.Expect(c.AddData([]byte(ext))).NotTo(gomega.HaveOccurred())
			}
			bs, err := c.MarshalYAML()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			extensions = append(extensions, string(bs))
		} else {
			extensions = append(extensions, extensionsByPeerID...)
		}
	}

	var resolvers []*Resolver
	// remove myself from the resolvers
	for _, r := range p.Resolvers {
		if r.Name != peer.Name {
			resolvers = append(resolvers, r)
		}
	}

	persistences := GetPersistences(peer.Options, p.NodeStorages(peer.UniqueName))

	t, err := template.New("peer").Funcs(template.FuncMap{
		"Replica":      func() *node2.Replica { return peer },
		"Peer":         func() *node2.Peer { return peer.Peer },
		"NetworkID":    func() string { return p.NetworkID },
		"Topology":     func() *Topology { return p.Topology },
		"Extensions":   func() []string { return extensions },
		"ToLower":      func(s string) string { return strings.ToLower(s) },
		"ReplaceAll":   func(s, old, new string) string { return strings.ReplaceAll(s, old, new) },
		"Persistences": func() map[driver.PersistenceName]node2.PersistenceOpts { return persistences },
		"Resolvers":    func() []*Resolver { return resolvers },
		"WebEnabled":   func() bool { return p.Topology.WebEnabled },
		"TracingEndpoint": func() string {
			return utils.DefaultString(p.Topology.Monitoring.TracingEndpoint, fmt.Sprintf("0.0.0.0:%d", otlp.JaegerCollectorPort))
		},
		"SamplingRatio": func() float64 { return p.Topology.Monitoring.TracingSamplingRatio },
	}).
		Parse(p.Topology.Templates.CoreTemplate())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(t.Execute(io.MultiWriter(core), p)).NotTo(gomega.HaveOccurred())

}

func GetPersistenceNames(o *node2.Options, prefixes ...node2.PersistenceKey) map[node2.PersistenceKey]driver.PersistenceName {
	names := make(map[node2.PersistenceKey]driver.PersistenceName, len(prefixes))
	// Complete all missing persistences with ad hoc sqlite local persistences
	for _, prefix := range prefixes {
		if name, ok := o.GetPersistenceName(prefix); !ok {
			names[prefix] = sqliteName(prefix)
		} else {
			names[prefix] = name
		}
	}

	return names
}

func GetPersistences(o *node2.Options, dir string) map[driver.PersistenceName]node2.PersistenceOpts {
	// First map all existing postgres configurations
	ps := o.GetPostgresPersistences()
	persistences := make(map[driver.PersistenceName]node2.PersistenceOpts, len(ps))
	for name, opt := range ps {
		persistences[name] = node2.PersistenceOpts{
			Type: postgres2.Persistence,
			SQL:  opt,
		}
	}

	// Complete all missing persistences with ad hoc sqlite local persistences
	prefixes := o.GetAllPersistenceKeys()
	for _, prefix := range prefixes {
		if _, ok := o.GetPersistenceName(prefix); !ok {
			persistences[sqliteName(prefix)] = node2.PersistenceOpts{
				Type: sqlite.Persistence,
				SQL: &node2.SQLOpts{
					DataSource: SqlitePath(dir, prefix),
				},
			}
		}
	}

	return persistences
}

func SqlitePath(storages string, prefix node2.PersistenceKey) string {
	return path.Join(storages, fmt.Sprintf("%s.sqlite", prefix))
}

func sqliteName(prefix node2.PersistenceKey) driver.PersistenceName {
	if driver.PersistenceName(prefix) == common2.DefaultPersistence {
		return common2.DefaultPersistence
	}
	return driver.PersistenceName(fmt.Sprintf("%s_persistence", prefix))
}

func (p *Platform) BootstrapViewNodeGroupRunner() ifrit.Runner {
	return grouper.NewParallel(syscall.SIGTERM, p.members(true))
}

func (p *Platform) FSCNodeGroupRunner() ifrit.Runner {
	return grouper.NewParallel(syscall.SIGTERM, p.members(false))
}

func (p *Platform) FSCNodeRunner(node *node2.Replica, env ...string) *runner2.Runner {
	// set config path
	env = append(env, fmt.Sprintf("FSCNODE_CFG_PATH=%s", p.NodeDir(node)))

	// enable/disable profiler
	profilerEnv := cmp.Or(os.Getenv("FSCNODE_PROFILER"), "FSCNODE_PROFILER=false")
	env = append(env, profilerEnv)

	cmd := p.fscNodeCommand(
		node,
		commands.NodeStart{NodeID: node.ID()},
		"",
		env...,
	)

	config := runner2.Config{
		AnsiColorCode:     common.NextColor(),
		Name:              node.ID(),
		Command:           cmd,
		StartCheck:        `Started peer with ID=.*`,
		StartCheckTimeout: 1 * time.Minute,
	}

	if p.Topology.LogToFile {
		logDir := filepath.Join(p.NodeDir(node), "logs")
		// set stdout to a file
		gomega.Expect(os.MkdirAll(logDir, 0755)).ToNot(gomega.HaveOccurred())
		f, err := os.Create(
			filepath.Join(
				logDir,
				fmt.Sprintf("%s.log", node.Name),
			),
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		config.Stdout = f
		config.Stderr = f
	}

	return runner2.New(config)
}

func (p *Platform) fscNodeCommand(node *node2.Replica, command common.Command, tlsDir string, env ...string) *exec.Cmd {
	if len(node.ExecutablePath) == 0 {
		node.ExecutablePath = p.GenerateCmd(nil, node)
	}
	cmd := common.NewCommand(p.Builder.Build(node.ExecutablePath), command)
	cmd.Env = append(cmd.Env, env...)
	cmd.Env = append(cmd.Env, "FSCNODE_LOGGING_SPEC="+p.Topology.Logging.Spec)
	if p.Context.IgnoreSigHUP() {
		cmd.Env = append(cmd.Env, "FSCNODE_SIGHUP_IGNORE=true")
	}
	// cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if p.Topology.GRPCLogging {
		cmd.Env = append(cmd.Env, "GRPC_GO_LOG_VERBOSITY_LEVEL=2")
		cmd.Env = append(cmd.Env, "GRPC_GO_LOG_SEVERITY_LEVEL=debug")
	}
	if common.ClientAuthEnabled(command) {
		certfilePath := filepath.Join(tlsDir, "client.crt")
		keyfilePath := filepath.Join(tlsDir, "client.key")

		cmd.Args = append(cmd.Args, "--certfile", certfilePath)
		cmd.Args = append(cmd.Args, "--keyfile", keyfilePath)
	}

	cmd.Env = append(cmd.Env, fmt.Sprintf("FSCNODE_LOGGING_SPEC=%s", p.Topology.Logging.Spec))

	return cmd
}

func (p *Platform) GenerateCmd(output io.Writer, node *node2.Replica) string {
	err := os.MkdirAll(p.NodeCmdDir(node), 0755)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	if output == nil {
		main, err := os.Create(p.NodeCmdPath(node))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		output = main
		defer utils.IgnoreErrorFunc(main.Close)
	}

	t, err := template.New("node").Funcs(template.FuncMap{
		"Alias":       func(s string) string { return node.Node.Alias(s) },
		"InstallView": func() bool { return len(node.Node.Responders) != 0 || len(node.Node.Factories) != 0 },
	}).Parse(p.Topology.Templates.NodeTemplate())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	gomega.Expect(t.Execute(io.MultiWriter(output), struct {
		*Platform
		*node2.Replica
	}{p, node})).NotTo(gomega.HaveOccurred())

	return p.NodeCmdPackage(node)
}

func (p *Platform) NodeDir(peer *node2.Replica) string {
	return filepath.Join(p.Context.RootDir(), "fsc", "nodes", peer.UniqueName)
}

func (p *Platform) NodeClientConfigPath(peer *node2.Replica) string {
	return filepath.Join(p.Context.RootDir(), "fsc", "nodes", peer.UniqueName, "client-config.yaml")
}

func (p *Platform) NodeStorages(uniqueName string) string {
	return filepath.Join(p.Context.RootDir(), "fsc", "nodes", uniqueName)
}

func (p *Platform) NodeStorageDir(uniqueName string, dirName string) string {
	return filepath.Join(p.NodeStorages(uniqueName), dirName)
}

func (p *Platform) NodeConfigPath(peer *node2.Replica) string {
	return filepath.Join(p.NodeDir(peer), "core.yaml")
}

func (p *Platform) NodeCmdDir(peer *node2.Replica) string {
	wd, err := os.Getwd()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return filepath.Join(wd, "out", "cmd", peer.Name)
}

func (p *Platform) NodeCmdPackage(peer *node2.Replica) string {
	gopath := os.Getenv("GOPATH")
	wd, err := os.Getwd()
	gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Failed to get working directory: %s", err)

	// if gopath is set to path within codebase, node command package will be relative within the codebase
	// if gopath is not set or not within codebase, then it will be absolute path where the fsc node code is
	// both can be built from these paths
	if withoutGoPath := strings.TrimPrefix(wd, filepath.Join(gopath, "src")); withoutGoPath != wd {
		return strings.TrimPrefix(
			filepath.Join(withoutGoPath, "out", "cmd", peer.Name),
			string(filepath.Separator),
		)
	}
	return filepath.Join(wd, "out", "cmd", peer.Name)
}

func (p *Platform) NodeCmdPath(peer *node2.Replica) string {
	return filepath.Join(p.NodeCmdDir(peer), "main.go")
}

func (p *Platform) NodePort(node *node2.Replica, portName api.PortName) uint16 {
	peerPorts := p.Context.PortsByPeerID("fsc", node.ID())
	gomega.Expect(peerPorts).NotTo(gomega.BeNil(), "cannot find ports for [%s][%v]", node.ID(), p.Context.PortsByPeerID)
	return peerPorts[portName]
}

func (p *Platform) BootstrapNode(me *node2.Peer) string {
	for _, node := range p.Topology.Nodes {
		if node.Bootstrap {
			if node.Name == me.Name {
				return ""
			}
			return node.Name
		}
	}
	return ""
}

func (p *Platform) ClientAuthRequired() bool {
	return false
}

func (p *Platform) CACertsBundlePath() string {
	return filepath.Join(p.Context.RootDir(), "fsc", "crypto", "ca-certs.pem")
}

func (p *Platform) NodeLocalTLSDir(peer *node2.Peer) string {
	return p.peerLocalCryptoDir(peer, "tls")
}

func (p *Platform) NodeLocalCertPath(node *node2.Peer) string {
	return p.LocalMSPIdentityCert(node)
}

func (p *Platform) NodeLocalPrivateKeyPath(node *node2.Peer) string {
	return p.LocalMSPPrivateKey(node)
}

func (p *Platform) LocalMSPIdentityCert(peer *node2.Peer) string {
	return filepath.Join(
		p.peerLocalCryptoDir(peer, "msp"),
		"signcerts",
		peer.Name+"."+p.Organization(peer.Organization).Domain+"-cert.pem",
	)
}

func (p *Platform) AdminLocalMSPIdentityCert(peer *node2.Peer) string {
	return filepath.Join(
		p.userLocalCryptoDir(peer, "Admin", "msp"),
		"signcerts",
		"Admin"+"@"+p.Organization(peer.Organization).Domain+"-cert.pem",
	)
}

func (p *Platform) LocalMSPPrivateKey(peer *node2.Peer) string {
	return filepath.Join(
		p.peerLocalCryptoDir(peer, "msp"),
		"keystore",
		"priv_sk",
	)
}

func (p *Platform) AdminLocalMSPPrivateKey(peer *node2.Peer) string {
	return filepath.Join(
		p.userLocalCryptoDir(peer, "Admin", "msp"),
		"keystore",
		"priv_sk",
	)
}

func (p *Platform) CryptoPath() string {
	return filepath.Join(p.Context.RootDir(), "fsc", "crypto")
}

func (p *Platform) Organization(orgName string) *node2.Organization {
	for _, org := range p.Organizations {
		if org.Name == orgName {
			return org
		}
	}
	return nil
}

func (p *Platform) ConcatenateTLSCACertificates() {
	bundle := &bytes.Buffer{}
	for _, tlsCertPath := range p.listTLSCACertificates() {
		certBytes, err := os.ReadFile(tlsCertPath)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		bundle.Write(certBytes)
	}
	if len(bundle.Bytes()) == 0 {
		return
	}

	err := os.WriteFile(p.CACertsBundlePath(), bundle.Bytes(), 0660)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (p *Platform) PeerOrgs() []*node2.Organization {
	orgsByName := map[string]*node2.Organization{}
	for _, peer := range p.Peers {
		orgsByName[peer.Organization] = p.Organization(peer.Organization)
	}

	var orgs []*node2.Organization
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

func (p *Platform) PeersInOrg(orgName string) []*node2.Peer {
	included := map[string]bool{}
	var peers []*node2.Peer
	for _, o := range p.Peers {
		if o.Organization == orgName && !included[o.Name] {
			peers = append(peers, o.Peer)
			included[o.Name] = true
		}
	}
	return peers
}

func (p *Platform) PeerAddress(peer *node2.Replica, portName api.PortName) string {
	return fmt.Sprintf("%s:%d", p.PeerHost(peer), p.PeerPort(peer, portName))
}

func (p *Platform) PeerHost(peer *node2.Replica) string {
	return p.Context.HostByPeerID("fsc", peer.ID())
}

func (p *Platform) GetReplicas(peer *node2.Peer) []*node2.Replica {
	uniqueNames := peer.ReplicaUniqueNames()
	replicas := make([]*node2.Replica, len(uniqueNames))
	for i, uniqueName := range uniqueNames {
		replicas[i] = &node2.Replica{Peer: peer, UniqueName: uniqueName}
	}
	return replicas
}

func (p *Platform) PeerPort(peer *node2.Replica, portName api.PortName) uint16 {
	peerPorts := p.Context.PortsByPeerID("fsc", peer.ID())
	gomega.Expect(peerPorts).NotTo(gomega.BeNil())
	return peerPorts[portName]
}

func (p *Platform) Peer(orgName, peerName string) *node2.Peer {
	for _, p := range p.PeersInOrg(orgName) {
		if p.Name == peerName {
			return p
		}
	}
	return nil
}

func (p *Platform) GetSigningIdentity(peer *node2.Peer) (client3.SigningIdentity, error) {
	return client3.NewX509SigningIdentity(p.LocalMSPIdentityCert(peer), p.LocalMSPPrivateKey(peer))
}

func (p *Platform) GetAdminSigningIdentity(peer *node2.Peer) (client3.SigningIdentity, error) {
	return client3.NewX509SigningIdentity(p.AdminLocalMSPIdentityCert(peer), p.AdminLocalMSPPrivateKey(peer))
}

func (p *Platform) listTLSCACertificates() []string {
	fileName2Path := make(map[string]string)
	gomega.Expect(filepath.Walk(filepath.Join(p.Context.RootDir(), "fsc", "crypto"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// File starts with "tlsca" and has "-cert.pem" in it
		if strings.HasPrefix(info.Name(), "tlsca") && strings.Contains(info.Name(), "-cert.pem") {
			fileName2Path[info.Name()] = path
		}
		return nil
	})).ToNot(gomega.HaveOccurred())

	var tlsCACertificates []string
	for _, path := range fileName2Path {
		tlsCACertificates = append(tlsCACertificates, path)
	}
	return tlsCACertificates
}

func (p *Platform) peerLocalCryptoDir(peer *node2.Peer, cryptoType string) string {
	org := p.Organization(peer.Organization)
	gomega.Expect(org).NotTo(gomega.BeNil())

	return filepath.Join(
		p.Context.RootDir(),
		"fsc",
		"crypto",
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", peer.Name, org.Domain),
		cryptoType,
	)
}

func (p *Platform) userLocalCryptoDir(peer *node2.Peer, user, cryptoMaterialType string) string {
	org := p.Organization(peer.Organization)
	gomega.Expect(org).NotTo(gomega.BeNil())

	return filepath.Join(
		p.Context.RootDir(),
		"fsc",
		"crypto",
		"peerOrganizations",
		org.Domain,
		"users",
		fmt.Sprintf("%s@%s", user, org.Domain),
		cryptoMaterialType,
	)
}

func (p *Platform) nextColor() string {
	color := p.colorIndex%14 + 31
	if color > 37 {
		color = color + 90 - 37
	}

	p.colorIndex++
	return fmt.Sprintf("%dm", color)
}

// PeerPortNames returns the list of ports that need to be reserved for a Peer.
func PeerPortNames() []api.PortName {
	return []api.PortName{ListenPort, P2PPort, WebPort}
}

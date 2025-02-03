/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"bytes"
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
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/optl"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	tracing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view/cmd"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/miracl/conflate"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/spf13/viper"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
)

var logger = logging.MustGetLogger("fsc.nwo")

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
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, p.EventuallyTimeout).Should(gexec.Exit(0))

	p.ConcatenateTLSCACertificates()

	p.GenerateResolverMap()

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
		Expect(c.ToFile(p.NodeClientConfigPath(peer))).ToNot(HaveOccurred())
	}

	// Generate commands
	for _, node := range p.Peers {
		if len(node.ExecutablePath) == 0 {
			p.GenerateCmd(nil, node)
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
	Expect(err).ToNot(HaveOccurred())
	p.Context.SetClientSigningIdentity(peer.Name, clientID)

	adminID, err := p.GetAdminSigningIdentity(peer.Peer)
	Expect(err).ToNot(HaveOccurred())
	p.Context.SetAdminSigningIdentity(peer.Name, adminID)

	cert, err := os.ReadFile(p.LocalMSPIdentityCert(peer.Peer))
	Expect(err).ToNot(HaveOccurred())
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
	// Start DBs
	configs := map[string]*postgres.ContainerConfig{}
	for _, node := range p.Peers {
		for _, sqlOpts := range node.Options.GetPersistences() {
			if sqlOpts.DriverType == sql.Postgres {
				if _, ok := configs[sqlOpts.DataSource]; !ok {
					c, err := postgres.ReadDataSource(sqlOpts.DataSource)
					Expect(err).ToNot(HaveOccurred())
					configs[sqlOpts.DataSource] = c
				}
			}
		}
	}
	logger.Infof("Starting DBs for following data sources: [%v]...", collections.Keys(configs))
	close, err := postgres.StartPostgresWithFmt(collections.Values(configs))
	Expect(err).ToNot(HaveOccurred(), "failed to start dbs")
	p.cleanDB = close
}

func (p *Platform) PostRun(bool) {
	for _, peer := range p.Peers {
		v := p.viper(peer)

		address := v.GetString("fsc.grpc.address")
		p.setIdentities(address, peer)
	}
	tracerProvider, err := tracing2.NewTracerProviderFromConfig(tracing2.Config{
		Provider: p.Topology.Monitoring.TracingType,
		File: tracing2.FileConfig{
			Path: "./client-trace.out",
		},
		Otpl: tracing2.OtplConfig{
			Address: fmt.Sprintf("0.0.0.0:%d", optl.JaegerCollectorPort),
		},
	})
	if err != nil {
		panic(err)
	}

	for _, node := range p.Peers {

		v := p.viper(node)

		// Prepare GRPC Client, Web Client, and CLI

		// GRPC client
		grpcClient, err := view.NewClient(
			&view.Config{
				ID:               v.GetString("fsc.id"),
				ConnectionConfig: p.Context.ConnectionConfig(node.UniqueName),
			},
			p.Context.ClientSigningIdentity(node.Name),
			crypto.NewProvider(),
			tracerProvider,
		)
		Expect(err).NotTo(HaveOccurred())
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
		Expect(err).NotTo(HaveOccurred())
		webClient, err := web.NewClient(webClientConfig)
		Expect(err).NotTo(HaveOccurred())
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
	Expect(err).NotTo(HaveOccurred())
	return v
}

func (p *Platform) Cleanup() {
	if p.metricsAggregatorProcess != nil {
		p.metricsAggregatorProcess.Signal(os.Kill)
	}
	p.cleanDB()
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
	fmt.Fprintf(
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
	Expect(os.MkdirAll(p.CryptoPath(), 0755)).NotTo(HaveOccurred())

	crypto, err := os.Create(p.CryptoConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer crypto.Close()

	t, err := template.New("crypto").Parse(p.Topology.Templates.CryptoTemplate())
	Expect(err).NotTo(HaveOccurred())

	Expect(t.Execute(io.MultiWriter(crypto), p)).NotTo(HaveOccurred())
}

func (p *Platform) GenerateRoutingConfig() {
	routing, err := os.Create(p.RoutingConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer routing.Close()

	t, err := template.New("routing").Parse(node2.RoutingTemplate)
	Expect(err).NotTo(HaveOccurred())

	Expect(t.Execute(io.MultiWriter(routing), p)).NotTo(HaveOccurred())
}

func (p *Platform) GenerateCoreConfig(peer *node2.Replica) {
	err := os.MkdirAll(p.NodeDir(peer), 0755)
	Expect(err).NotTo(HaveOccurred())

	core, err := os.Create(p.NodeConfigPath(peer))
	Expect(err).NotTo(HaveOccurred())
	defer core.Close()

	var extensions []string
	for _, extensionsByPeerID := range p.Context.ExtensionsByPeerID(peer.UniqueName) {
		// if len(extensionsByPeerID) > 1, we need a merge
		if len(extensionsByPeerID) > 1 {
			c := conflate.New()
			for _, ext := range extensionsByPeerID {
				Expect(c.AddData([]byte(ext))).NotTo(HaveOccurred())
			}
			bs, err := c.MarshalYAML()
			Expect(err).NotTo(HaveOccurred())
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

	t, err := template.New("peer").Funcs(template.FuncMap{
		"Replica":    func() *node2.Replica { return peer },
		"Peer":       func() *node2.Peer { return peer.Peer },
		"NetworkID":  func() string { return p.NetworkID },
		"Topology":   func() *Topology { return p.Topology },
		"Extensions": func() []string { return extensions },
		"ToLower":    func(s string) string { return strings.ToLower(s) },
		"ReplaceAll": func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
		"KVSOpts": func() node2.PersistenceOpts {
			return PersistenceOpts(KvsPersistencePrefix, peer.Options, p.NodeStorageDir(peer.UniqueName, "kvs"))
		},
		"BindingOpts": func() node2.PersistenceOpts {
			return PersistenceOpts(BindingPersistencePrefix, peer.Options, p.NodeStorageDir(peer.UniqueName, "bind"))
		},
		"SignerInfoOpts": func() node2.PersistenceOpts {
			return PersistenceOpts(SignerInfoPersistencePrefix, peer.Options, p.NodeStorageDir(peer.UniqueName, "sig"))
		},
		"AuditInfoOpts": func() node2.PersistenceOpts {
			return PersistenceOpts(AuditInfoPersistencePrefix, peer.Options, p.NodeStorageDir(peer.UniqueName, "aud"))
		},
		"EndorseTxOpts": func() node2.PersistenceOpts {
			return PersistenceOpts(EndorseTxPersistencePrefix, peer.Options, p.NodeStorageDir(peer.UniqueName, "etx"))
		},
		"EnvelopeOpts": func() node2.PersistenceOpts {
			return PersistenceOpts(EnvelopePersistencePrefix, peer.Options, p.NodeStorageDir(peer.UniqueName, "env"))
		},
		"MetadataOpts": func() node2.PersistenceOpts {
			return PersistenceOpts(MetadataPersistencePrefix, peer.Options, p.NodeStorageDir(peer.UniqueName, "mtd"))
		},
		"Resolvers":  func() []*Resolver { return resolvers },
		"WebEnabled": func() bool { return p.Topology.WebEnabled },
		"TracingEndpoint": func() string {
			return utils.DefaultString(p.Topology.Monitoring.TracingEndpoint, fmt.Sprintf("0.0.0.0:%d", optl.JaegerCollectorPort))
		},
		"SamplingRatio": func() float64 { return p.Topology.Monitoring.TracingSamplingRatio },
	}).
		Parse(p.Topology.Templates.CoreTemplate())
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Execute(io.MultiWriter(core), p)).NotTo(HaveOccurred())

}

func PersistenceOpts(prefix string, o *node2.Options, dir string) node2.PersistenceOpts {
	if sqlOpts := o.GetPersistence(prefix); sqlOpts != nil {
		return node2.PersistenceOpts{
			Type: sql.SQLPersistence,
			SQL:  sqlOpts,
		}
	}
	return node2.PersistenceOpts{
		Type: sql.SQLPersistence,
		SQL: &node2.SQLOpts{
			DriverType:   sql.SQLite,
			DataSource:   fmt.Sprintf("%s.sqlite", dir),
			CreateSchema: true,
		}}
}

func (p *Platform) BootstrapViewNodeGroupRunner() ifrit.Runner {
	return grouper.NewParallel(syscall.SIGTERM, p.members(true))
}

func (p *Platform) FSCNodeGroupRunner() ifrit.Runner {
	return grouper.NewParallel(syscall.SIGTERM, p.members(false))
}

func (p *Platform) FSCNodeRunner(node *node2.Replica, env ...string) *runner2.Runner {
	cmd := p.fscNodeCommand(
		node,
		commands.NodeStart{NodeID: node.ID()},
		"",
		fmt.Sprintf("FSCNODE_CFG_PATH=%s", p.NodeDir(node)),
		"FSCNODE_PROFILER=true",
	)
	cmd.Env = append(cmd.Env, env...)

	config := runner2.Config{
		AnsiColorCode:     common.NextColor(),
		Name:              node.ID(),
		Command:           cmd,
		StartCheck:        `Started peer with ID=.*, address=`,
		StartCheckTimeout: 1 * time.Minute,
	}

	if p.Topology.LogToFile {
		logDir := filepath.Join(p.NodeDir(node), "logs")
		// set stdout to a file
		Expect(os.MkdirAll(logDir, 0755)).ToNot(HaveOccurred())
		f, err := os.Create(
			filepath.Join(
				logDir,
				fmt.Sprintf("%s.log", node.Name),
			),
		)
		Expect(err).ToNot(HaveOccurred())
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
	Expect(err).NotTo(HaveOccurred())

	if output == nil {
		main, err := os.Create(p.NodeCmdPath(node))
		Expect(err).NotTo(HaveOccurred())
		output = main
		defer main.Close()
	}

	t, err := template.New("node").Funcs(template.FuncMap{
		"Alias":       func(s string) string { return node.Node.Alias(s) },
		"InstallView": func() bool { return len(node.Node.Responders) != 0 || len(node.Node.Factories) != 0 },
	}).Parse(p.Topology.Templates.NodeTemplate())
	Expect(err).NotTo(HaveOccurred())

	Expect(t.Execute(io.MultiWriter(output), struct {
		*Platform
		*node2.Replica
	}{p, node})).NotTo(HaveOccurred())

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
	Expect(err).ToNot(HaveOccurred())

	return filepath.Join(wd, "cmd", peer.Name)
}

func (p *Platform) NodeCmdPackage(peer *node2.Replica) string {
	gopath := os.Getenv("GOPATH")
	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred(), "Failed to get working directory: %s", err)

	// if gopath is set to path within codebase, node command package will be relative within the codebase
	// if gopath is not set or not within codebase, then it will be absolute path where the fsc node code is
	// both can be built from these paths
	if withoutGoPath := strings.TrimPrefix(wd, filepath.Join(gopath, "src")); withoutGoPath != wd {
		return strings.TrimPrefix(
			filepath.Join(withoutGoPath, "cmd", peer.Name),
			string(filepath.Separator),
		)
	}
	return filepath.Join(wd, "cmd", peer.Name)
}

func (p *Platform) NodeCmdPath(peer *node2.Replica) string {
	return filepath.Join(p.NodeCmdDir(peer), "main.go")
}

func (p *Platform) NodePort(node *node2.Replica, portName api.PortName) uint16 {
	peerPorts := p.Context.PortsByPeerID("fsc", node.ID())
	Expect(peerPorts).NotTo(BeNil(), "cannot find ports for [%s][%v]", node.ID(), p.Context.PortsByPeerID)
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
		Expect(err).NotTo(HaveOccurred())
		bundle.Write(certBytes)
	}
	if len(bundle.Bytes()) == 0 {
		return
	}

	err := os.WriteFile(p.CACertsBundlePath(), bundle.Bytes(), 0660)
	Expect(err).NotTo(HaveOccurred())
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
	Expect(peerPorts).NotTo(BeNil())
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

func (p *Platform) GetSigningIdentity(peer *node2.Peer) (view.SigningIdentity, error) {
	return view.NewX509SigningIdentity(p.LocalMSPIdentityCert(peer), p.LocalMSPPrivateKey(peer))
}

func (p *Platform) GetAdminSigningIdentity(peer *node2.Peer) (view.SigningIdentity, error) {
	return view.NewX509SigningIdentity(p.AdminLocalMSPIdentityCert(peer), p.AdminLocalMSPPrivateKey(peer))
}

func (p *Platform) listTLSCACertificates() []string {
	fileName2Path := make(map[string]string)
	Expect(filepath.Walk(filepath.Join(p.Context.RootDir(), "fsc", "crypto"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// File starts with "tlsca" and has "-cert.pem" in it
		if strings.HasPrefix(info.Name(), "tlsca") && strings.Contains(info.Name(), "-cert.pem") {
			fileName2Path[info.Name()] = path
		}
		return nil
	})).ToNot(HaveOccurred())

	var tlsCACertificates []string
	for _, path := range fileName2Path {
		tlsCACertificates = append(tlsCACertificates, path)
	}
	return tlsCACertificates
}

func (p *Platform) peerLocalCryptoDir(peer *node2.Peer, cryptoType string) string {
	org := p.Organization(peer.Organization)
	Expect(org).NotTo(BeNil())

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
	Expect(org).NotTo(BeNil())

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

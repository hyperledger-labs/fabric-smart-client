/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fsc

import (
	"fmt"
	"go/build"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"
	"time"

	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/registry"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/runner"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client"
)

const (
	ListenPort registry.PortName = "Listen" // Port at which the fsc node might listen for some service
	ViewPort   registry.PortName = "View"   // Port at which the View Service Server respond
	P2PPort    registry.PortName = "P2P"    // Port at which the P2P Communication Layer respond
)

type Builder interface {
	Build(path string) string
}

type platform struct {
	Registry *registry.Registry
	Builder  Builder
	Topology *Topology
}

func NewPlatform(Registry *registry.Registry, Builder Builder) *platform {
	return &platform{
		Registry: Registry,
		Builder:  Builder,
		Topology: Registry.TopologyByName(TopologyName).(*Topology),
	}
}

func (p *platform) Name() string {
	return TopologyName
}

func (p *platform) GenerateConfigTree() {
	// Allocations
	bootstrapNodeFound := false
	for _, node := range p.Topology.Nodes {
		// Reserve ports
		ports := registry.Ports{}
		for _, portName := range PeerPortNames() {
			ports[portName] = p.Registry.ReservePort()
		}
		p.Registry.PortsByPeerID[node.ID()] = ports

		// Is this a bootstrap node/
		if node.Bootstrap {
			bootstrapNodeFound = true
		}
	}
	if !bootstrapNodeFound {
		p.Topology.Nodes[0].Bootstrap = true
	}

}

func (p *platform) GenerateArtifacts() {
	// Generate core.yaml for all fsc nodes by including all the additional configurations coming
	// from other platforms
	for _, node := range p.Topology.Nodes {
		p.GenerateCoreConfig(node)
	}
}

func (p *platform) Load() {
	// Nothing to do here
}

func (p *platform) Members() []grouper.Member {
	members := grouper.Members{}
	for _, node := range p.Topology.Nodes {
		if node.Bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.ViewNodeRunner(node)})
		}
	}
	for _, node := range p.Topology.Nodes {
		if !node.Bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.ViewNodeRunner(node)})
		}
	}
	return members
}

func (p *platform) PostRun() {
	for _, node := range p.Topology.Nodes {
		v := viper.New()
		v.SetConfigFile(p.NodeConfigPath(node))
		err := v.ReadInConfig() // Find and read the config file
		Expect(err).NotTo(HaveOccurred())

		// Get from the registry the signing identity and the connection config
		c, err := client.New(
			&client.Config{
				ID:      v.GetString("fsc.id"),
				FSCNode: p.Registry.ConnectionConfigs[node.Name],
			},
			p.Registry.ClientSigningIdentities[node.Name],
			crypto.NewProvider(),
		)
		Expect(err).NotTo(HaveOccurred())

		p.Registry.ViewClients[node.ID()] = c
		for _, identity := range p.Registry.ViewIdentityAliases[node.ID()] {
			p.Registry.ViewClients[identity] = c
		}
	}
}

func (p *platform) Cleanup() {
}

func (p *platform) GenerateCoreConfig(peer *Node) {
	err := os.MkdirAll(p.NodeDir(peer), 0755)
	Expect(err).NotTo(HaveOccurred())

	core, err := os.Create(p.NodeConfigPath(peer))
	Expect(err).NotTo(HaveOccurred())
	defer core.Close()

	var extensions []string
	for _, ext := range p.Registry.ExtensionsByPeerID[peer.Name] {
		extensions = append(extensions, ext)
	}

	t, err := template.New("peer").Funcs(template.FuncMap{
		"Peer":          func() *Node { return peer },
		"Registry":      func() *registry.Registry { return p.Registry },
		"FabricEnabled": func() bool { return p.Registry.TopologyByName("fabric") != nil },
		"Topology":      func() *Topology { return p.Topology },
		"Extensions":    func() []string { return extensions },
		"ToLower":       func(s string) string { return strings.ToLower(s) },
		"ReplaceAll":    func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
		"NodeKVSPath":   func() string { return p.NodeKVSDir(peer) },
	}).Parse(CoreTemplate)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Execute(io.MultiWriter(core), p)).NotTo(HaveOccurred())
}

func (p *platform) BootstrapViewNodeGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	for _, node := range p.Topology.Nodes {
		if node.Bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.ViewNodeRunner(node)})
		}
	}
	return runner.NewParallel(syscall.SIGTERM, members)
}

func (p *platform) ViewNodeGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	for _, node := range p.Topology.Nodes {
		if !node.Bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.ViewNodeRunner(node)})
		}
	}
	return runner.NewParallel(syscall.SIGTERM, members)
}

func (p *platform) ViewNodeRunner(node *Node, env ...string) *runner.Runner {
	cmd := p.fscNodeCommand(
		node,
		commands.NodeStart{NodeID: node.ID()},
		"",
		fmt.Sprintf("FSCNODE_CFG_PATH=%s", p.NodeDir(node)),
	)
	cmd.Env = append(cmd.Env, env...)

	return runner.New(runner.Config{
		AnsiColorCode:     common.NextColor(),
		Name:              node.ID(),
		Command:           cmd,
		StartCheck:        `Started peer with ID=.*, .*, address=`,
		StartCheckTimeout: 1 * time.Minute,
	})
}

func (p *platform) fscNodeCommand(node *Node, command common.Command, tlsDir string, env ...string) *exec.Cmd {
	if len(node.ExecutablePath) == 0 {
		node.ExecutablePath = p.GenerateCmd(nil, node)
	}
	cmd := common.NewCommand(p.Builder.Build(node.ExecutablePath), command)
	cmd.Env = append(cmd.Env, env...)
	cmd.Env = append(cmd.Env, "FSCNODE_LOGGING_SPEC="+p.Topology.Logging.Spec)

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

	cmd.Args = append(cmd.Args, "--logging-level", p.Topology.Logging.Spec)

	return cmd
}

func (p *platform) GenerateCmd(output io.Writer, node *Node) string {
	err := os.MkdirAll(p.NodeCmdDir(node), 0755)
	Expect(err).NotTo(HaveOccurred())

	if output == nil {
		main, err := os.Create(p.NodeCmdPath(node))
		Expect(err).NotTo(HaveOccurred())
		output = main
		defer main.Close()
	}

	t, err := template.New("node").Funcs(template.FuncMap{
		"Alias":       func(s string) string { return node.Alias(s) },
		"InstallView": func() bool { return len(node.Responders) != 0 || len(node.Factories) != 0 },
	}).Parse(DefaultTemplate)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Execute(io.MultiWriter(output), node)).NotTo(HaveOccurred())

	return p.NodeCmdPackage(node)
}

func (p *platform) NodeDir(peer *Node) string {
	return filepath.Join(p.Registry.RootDir, "fscnodes", peer.ID())
}

func (p *platform) NodeKVSDir(peer *Node) string {
	return filepath.Join(p.Registry.RootDir, "fscnodes", peer.ID(), "kvs")
}

func (p *platform) NodeConfigPath(peer *Node) string {
	return filepath.Join(p.NodeDir(peer), "core.yaml")
}

func (p *platform) NodeCmdDir(peer *Node) string {
	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	return filepath.Join(wd, "cmd", peer.Name)
}

func (p *platform) NodeCmdPackage(peer *Node) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = build.Default.GOPATH
	}
	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	return strings.TrimPrefix(
		filepath.Join(strings.TrimPrefix(wd, filepath.Join(gopath, "src")), "cmd", peer.Name),
		string(filepath.Separator),
	)
}

func (p *platform) NodeCmdPath(peer *Node) string {
	return filepath.Join(p.NodeCmdDir(peer), "main.go")
}

func (p *platform) NodePort(node *Node, portName registry.PortName) uint16 {
	peerPorts := p.Registry.PortsByPeerID[node.ID()]
	Expect(peerPorts).NotTo(BeNil())
	return peerPorts[portName]
}

func (p *platform) BootstrapNode(me *Node) string {
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

func (p *platform) ClientAuthRequired() bool {
	return false
}

func (p *platform) CACertsBundlePath() string {
	return filepath.Join(p.Registry.RootDir, "crypto", "ca-certs.pem")
}

func (p *platform) NodeLocalTLSDir(node *Node) string {
	return node.Options.Mapping["NodeLocalTLSDir"].(string)
}

func (p *platform) NodeLocalCertPath(node *Node) string {
	return node.Options.Mapping["NodeLocalCertPath"].(string)
}

func (p *platform) NodeLocalPrivateKeyPath(node *Node) string {
	return node.Options.Mapping["NodeLocalPrivateKeyPath"].(string)
}

// PeerPortNames returns the list of ports that need to be reserved for a Peer.
func PeerPortNames() []registry.PortName {
	return []registry.PortName{ListenPort, P2PPort}
}

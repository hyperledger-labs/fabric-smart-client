/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/prometheus/common/log"
	"github.com/spf13/viper"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"gopkg.in/yaml.v2"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	runner2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/identity"
	node2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

const (
	ListenPort api.PortName = "Listen" // Port at which the fsc node might listen for some service
	ViewPort   api.PortName = "View"   // Port at which the View Service Server respond
	P2PPort    api.PortName = "P2P"    // Port at which the P2P Communication Layer respond
)

type platform struct {
	Context           api.Context
	NetworkID         string
	Builder           *Builder
	Topology          *Topology
	EventuallyTimeout time.Duration

	Organizations []*node2.Organization
	Peers         []*node2.Peer
	Resolvers     []*Resolver
	colorIndex    int
}

func NewPlatform(Registry api.Context, t api.Topology, builderClient BuilderClient) *platform {
	p := &platform{
		Context:           Registry,
		NetworkID:         common.UniqueName(),
		Builder:           &Builder{client: builderClient},
		Topology:          t.(*Topology),
		EventuallyTimeout: 10 * time.Minute,
	}
	p.CheckTopology()
	return p
}

func (p *platform) Name() string {
	return TopologyName
}

func (p *platform) Type() string {
	return TopologyName
}

func (p *platform) GenerateConfigTree() {
	p.GenerateCryptoConfig()
}

func (p *platform) GenerateArtifacts() {
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
		cc := &grpc.ConnectionConfig{
			Address:           p.PeerAddress(peer, ListenPort),
			TLSEnabled:        true,
			TLSRootCertFile:   path.Join(p.NodeLocalTLSDir(peer), "ca.crt"),
			ConnectionTimeout: 10 * time.Minute,
		}
		p.Context.SetConnectionConfig(peer.Name, cc)

		clientID, err := p.GetSigningIdentity(peer)
		Expect(err).ToNot(HaveOccurred())
		p.Context.SetClientSigningIdentity(peer.Name, clientID)

		adminID, err := p.GetAdminSigningIdentity(peer)
		Expect(err).ToNot(HaveOccurred())
		p.Context.SetAdminSigningIdentity(peer.Name, adminID)

		cert, err := ioutil.ReadFile(p.LocalMSPIdentityCert(peer))
		Expect(err).ToNot(HaveOccurred())
		p.Context.SetViewIdentity(peer.Name, cert)

		p.GenerateCoreConfig(peer)
	}
}

func (p *platform) Load() {
	for _, peer := range p.Peers {
		v := viper.New()
		v.SetConfigFile(p.NodeConfigPath(peer))
		err := v.ReadInConfig() // Find and read the config file
		Expect(err).NotTo(HaveOccurred())

		cc := &grpc.ConnectionConfig{
			Address:           v.GetString("fsc.address"),
			TLSEnabled:        true,
			TLSRootCertFile:   path.Join(p.NodeLocalTLSDir(peer), "ca.crt"),
			ConnectionTimeout: 10 * time.Minute,
		}
		p.Context.SetConnectionConfig(peer.Name, cc)

		clientID, err := p.GetSigningIdentity(peer)
		Expect(err).ToNot(HaveOccurred())
		p.Context.SetClientSigningIdentity(peer.Name, clientID)

		adminID, err := p.GetAdminSigningIdentity(peer)
		Expect(err).ToNot(HaveOccurred())
		p.Context.SetAdminSigningIdentity(peer.Name, adminID)

		cert, err := ioutil.ReadFile(p.LocalMSPIdentityCert(peer))
		Expect(err).ToNot(HaveOccurred())
		p.Context.SetViewIdentity(peer.Name, cert)
	}
}

func (p *platform) Members() []grouper.Member {
	members := grouper.Members{}
	for _, node := range p.Peers {
		if node.Bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.FSCNodeRunner(node)})
		}
	}
	for _, node := range p.Peers {
		if !node.Bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.FSCNodeRunner(node)})
		}
	}
	return members
}

func (p *platform) PostRun() {
	for _, node := range p.Peers {
		v := viper.New()
		v.SetConfigFile(p.NodeConfigPath(node))
		err := v.ReadInConfig() // Find and read the config file
		Expect(err).NotTo(HaveOccurred())

		// Get from the registry the signing identity and the connection config
		c, err := view.New(
			&view.Config{
				ID:      v.GetString("fsc.id"),
				FSCNode: p.Context.ConnectionConfig(node.Name),
			},
			p.Context.ClientSigningIdentity(node.Name),
			crypto.NewProvider(),
		)
		Expect(err).NotTo(HaveOccurred())

		p.Context.SetViewClient(node.Name, c)
		p.Context.SetViewClient(node.ID(), c)
		for _, identity := range p.Context.GetViewIdentityAliases(node.ID()) {
			p.Context.SetViewClient(identity, c)
		}
		for _, identity := range p.Context.GetViewIdentityAliases(node.Name) {
			p.Context.SetViewClient(identity, c)
		}
		for _, alias := range node.Aliases {
			p.Context.SetViewClient(alias.Alias, c)
		}

		// Setup admins
		if id := p.Context.AdminSigningIdentity(node.Name); id != nil {
			c, err := view.New(
				&view.Config{
					ID:      v.GetString("fsc.id"),
					FSCNode: p.Context.ConnectionConfig(node.Name),
				},
				id,
				crypto.NewProvider(),
			)
			Expect(err).NotTo(HaveOccurred())

			p.Context.SetViewClient(node.Name+".admin", c)
			p.Context.SetViewClient(node.ID()+".admin", c)
		}
	}
}

func (p *platform) Cleanup() {
}

func (p *platform) CheckTopology() {
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
		var extraIdentities []*node2.PeerIdentity
		peer := &node2.Peer{
			Name:            node.Name,
			Organization:    org.Name,
			Bootstrap:       node.Bootstrap,
			ExecutablePath:  node.ExecutablePath,
			ExtraIdentities: extraIdentities,
			Node:            node,
		}
		peer.Admins = []string{
			p.AdminLocalMSPIdentityCert(peer),
		}
		p.Peers = append(p.Peers, peer)
		ports := api.Ports{}
		for _, portName := range PeerPortNames() {
			ports[portName] = p.Context.ReservePort()
		}
		p.Context.SetPortsByPeerID("fsc", peer.ID(), ports)
		users[orgName] = users[orgName] + 1
		userNames[orgName] = append(userNames[orgName], node.Name)

		// Is this a bootstrap node/
		if node.Bootstrap {
			bootstrapNodeFound = true
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

func (p *platform) Cryptogen(command common.Command) (*gexec.Session, error) {
	cmd := common.NewCommand(p.Builder.Cryptogen(), command)
	return p.StartSession(cmd, command.SessionName())
}

func (p *platform) StartSession(cmd *exec.Cmd, name string) (*gexec.Session, error) {
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

func (p *platform) CryptoConfigPath() string {
	return filepath.Join(p.Context.RootDir(), "fsc", "crypto-config.yaml")
}

func (p *platform) GenerateCryptoConfig() {
	Expect(os.MkdirAll(p.CryptoPath(), 0755)).NotTo(HaveOccurred())

	crypto, err := os.Create(p.CryptoConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer crypto.Close()

	t, err := template.New("crypto").Parse(node2.DefaultCryptoTemplate)
	Expect(err).NotTo(HaveOccurred())

	Expect(t.Execute(io.MultiWriter(crypto), p)).NotTo(HaveOccurred())
}

func (p *platform) GenerateCoreConfig(peer *node2.Peer) {
	err := os.MkdirAll(p.NodeDir(peer), 0755)
	Expect(err).NotTo(HaveOccurred())

	core, err := os.Create(p.NodeConfigPath(peer))
	Expect(err).NotTo(HaveOccurred())
	defer core.Close()

	var extensions []string
	for _, extensionsByPeerID := range p.Context.ExtensionsByPeerID(peer.Name) {
		// if len(extensionsByPeerID) > 1, we need a merge
		if len(extensionsByPeerID) > 1 {
			// merge
			var resultValues map[string]interface{}
			for _, ext := range extensionsByPeerID {
				var override map[string]interface{}
				if err := yaml.Unmarshal([]byte(ext), &override); err != nil {
					log.Info(err)
					continue
				}

				// we expect override to have a single root key
				if resultValues == nil {
					resultValues = override
				} else {
					// merge the single root key
					for k, v := range override {
						// merge resultValues[k] and v
						m1 := resultValues[k].(map[interface{}]interface{})
						m2 := v.(map[interface{}]interface{})

						for kk, vv := range m2 {
							m1[kk] = vv
						}
					}
				}
			}
			bs, err := yaml.Marshal(resultValues)
			if err != nil {
				panic(err)
			}
			extensions = append(extensions, string(bs))
		} else {
			for _, s := range extensionsByPeerID {
				extensions = append(extensions, s)
			}
		}
	}

	t, err := template.New("peer").Funcs(template.FuncMap{
		"Peer":        func() *node2.Peer { return peer },
		"NetworkID":   func() string { return p.NetworkID },
		"Topology":    func() *Topology { return p.Topology },
		"Extensions":  func() []string { return extensions },
		"ToLower":     func(s string) string { return strings.ToLower(s) },
		"ReplaceAll":  func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
		"NodeKVSPath": func() string { return p.NodeKVSDir(peer) },
	}).Parse(node2.CoreTemplate)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Execute(io.MultiWriter(core), p)).NotTo(HaveOccurred())
}

func (p *platform) BootstrapViewNodeGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	for _, node := range p.Peers {
		if node.Bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.FSCNodeRunner(node)})
		}
	}
	return runner2.NewParallel(syscall.SIGTERM, members)
}

func (p *platform) FSCNodeGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	for _, node := range p.Peers {
		if !node.Bootstrap {
			members = append(members, grouper.Member{Name: node.ID(), Runner: p.FSCNodeRunner(node)})
		}
	}
	return runner2.NewParallel(syscall.SIGTERM, members)
}

func (p *platform) FSCNodeRunner(node *node2.Peer, env ...string) *runner2.Runner {
	cmd := p.fscNodeCommand(
		node,
		commands.NodeStart{NodeID: node.ID()},
		"",
		fmt.Sprintf("FSCNODE_CFG_PATH=%s", p.NodeDir(node)),
	)
	cmd.Env = append(cmd.Env, env...)

	return runner2.New(runner2.Config{
		AnsiColorCode:     common.NextColor(),
		Name:              node.ID(),
		Command:           cmd,
		StartCheck:        `Started peer with ID=.*, .*, address=`,
		StartCheckTimeout: 1 * time.Minute,
	})
}

func (p *platform) fscNodeCommand(node *node2.Peer, command common.Command, tlsDir string, env ...string) *exec.Cmd {
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

func (p *platform) GenerateCmd(output io.Writer, node *node2.Peer) string {
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
	}).Parse(node2.DefaultTemplate)
	Expect(err).NotTo(HaveOccurred())
	Expect(t.Execute(io.MultiWriter(output), node)).NotTo(HaveOccurred())

	return p.NodeCmdPackage(node)
}

func (p *platform) NodeDir(peer *node2.Peer) string {
	return filepath.Join(p.Context.RootDir(), "fsc", "fscnodes", peer.ID())
}

func (p *platform) NodeKVSDir(peer *node2.Peer) string {
	return filepath.Join(p.Context.RootDir(), "fsc", "fscnodes", peer.ID(), "kvs")
}

func (p *platform) NodeConfigPath(peer *node2.Peer) string {
	return filepath.Join(p.NodeDir(peer), "core.yaml")
}

func (p *platform) NodeCmdDir(peer *node2.Peer) string {
	wd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	return filepath.Join(wd, "cmd", peer.Name)
}

func (p *platform) NodeCmdPackage(peer *node2.Peer) string {
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

func (p *platform) NodeCmdPath(peer *node2.Peer) string {
	return filepath.Join(p.NodeCmdDir(peer), "main.go")
}

func (p *platform) NodePort(node *node2.Peer, portName api.PortName) uint16 {
	peerPorts := p.Context.PortsByPeerID("fsc", node.ID())
	Expect(peerPorts).NotTo(BeNil(), "cannot find ports for [%s][%v]", node.ID(), p.Context.PortsByPeerID)
	return peerPorts[portName]
}

func (p *platform) BootstrapNode(me *node2.Peer) string {
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
	return filepath.Join(p.Context.RootDir(), "fsc", "crypto", "ca-certs.pem")
}

func (p *platform) NodeLocalTLSDir(peer *node2.Peer) string {
	return p.peerLocalCryptoDir(peer, "tls")
}

func (p *platform) NodeLocalCertPath(node *node2.Peer) string {
	return p.LocalMSPIdentityCert(node)
}

func (p *platform) NodeLocalPrivateKeyPath(node *node2.Peer) string {
	return p.LocalMSPPrivateKey(node)
}

func (p *platform) LocalMSPIdentityCert(peer *node2.Peer) string {
	return filepath.Join(
		p.peerLocalCryptoDir(peer, "msp"),
		"signcerts",
		peer.Name+"."+p.Organization(peer.Organization).Domain+"-cert.pem",
	)
}

func (p *platform) AdminLocalMSPIdentityCert(peer *node2.Peer) string {
	return filepath.Join(
		p.userLocalCryptoDir(peer, "Admin", "msp"),
		"signcerts",
		"Admin"+"@"+p.Organization(peer.Organization).Domain+"-cert.pem",
	)
}

func (p *platform) LocalMSPPrivateKey(peer *node2.Peer) string {
	return filepath.Join(
		p.peerLocalCryptoDir(peer, "msp"),
		"keystore",
		"priv_sk",
	)
}

func (p *platform) AdminLocalMSPPrivateKey(peer *node2.Peer) string {
	return filepath.Join(
		p.userLocalCryptoDir(peer, "Admin", "msp"),
		"keystore",
		"priv_sk",
	)
}

func (p *platform) CryptoPath() string {
	return filepath.Join(p.Context.RootDir(), "fsc", "crypto")
}

func (p *platform) Organization(orgName string) *node2.Organization {
	for _, org := range p.Organizations {
		if org.Name == orgName {
			return org
		}
	}
	return nil
}

func (p *platform) ConcatenateTLSCACertificates() {
	bundle := &bytes.Buffer{}
	for _, tlsCertPath := range p.listTLSCACertificates() {
		certBytes, err := ioutil.ReadFile(tlsCertPath)
		Expect(err).NotTo(HaveOccurred())
		bundle.Write(certBytes)
	}
	if len(bundle.Bytes()) == 0 {
		return
	}

	err := ioutil.WriteFile(p.CACertsBundlePath(), bundle.Bytes(), 0660)
	Expect(err).NotTo(HaveOccurred())
}

func (p *platform) PeerOrgs() []*node2.Organization {
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

func (p *platform) PeersInOrg(orgName string) []*node2.Peer {
	var peers []*node2.Peer
	for _, o := range p.Peers {
		if o.Organization == orgName {
			peers = append(peers, o)
		}
	}
	return peers
}

func (p *platform) PeerAddress(peer *node2.Peer, portName api.PortName) string {
	return fmt.Sprintf("127.0.0.1:%d", p.PeerPort(peer, portName))
}

func (p *platform) PeerPort(peer *node2.Peer, portName api.PortName) uint16 {
	peerPorts := p.Context.PortsByPeerID("fsc", peer.ID())
	Expect(peerPorts).NotTo(BeNil())
	return peerPorts[portName]
}

func (p *platform) Peer(orgName, peerName string) *node2.Peer {
	for _, p := range p.PeersInOrg(orgName) {
		if p.Name == peerName {
			return p
		}
	}
	return nil
}

func (p *platform) GetSigningIdentity(peer *node2.Peer) (identity.SigningIdentity, error) {
	cert, err := ioutil.ReadFile(p.LocalMSPIdentityCert(peer))
	if err != nil {
		return nil, err
	}
	sk, err := ioutil.ReadFile(p.LocalMSPPrivateKey(peer))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(sk)
	if block == nil {
		return nil, fmt.Errorf("failed decoding PEM. Block must be different from nil. [% x]", sk)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return identity.NewSigningIdentity(cert, k.(*ecdsa.PrivateKey)), nil
}

func (p *platform) GetAdminSigningIdentity(peer *node2.Peer) (identity.SigningIdentity, error) {
	cert, err := ioutil.ReadFile(p.AdminLocalMSPIdentityCert(peer))
	if err != nil {
		return nil, err
	}
	sk, err := ioutil.ReadFile(p.AdminLocalMSPPrivateKey(peer))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(sk)
	if block == nil {
		return nil, fmt.Errorf("failed decoding PEM. Block must be different from nil. [% x]", sk)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return identity.NewSigningIdentity(cert, k.(*ecdsa.PrivateKey)), nil
}

func (p *platform) listTLSCACertificates() []string {
	fileName2Path := make(map[string]string)
	filepath.Walk(filepath.Join(p.Context.RootDir(), "fsc", "crypto"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// File starts with "tlsca" and has "-cert.pem" in it
		if strings.HasPrefix(info.Name(), "tlsca") && strings.Contains(info.Name(), "-cert.pem") {
			fileName2Path[info.Name()] = path
		}
		return nil
	})

	var tlsCACertificates []string
	for _, path := range fileName2Path {
		tlsCACertificates = append(tlsCACertificates, path)
	}
	return tlsCACertificates
}

func (p *platform) peerLocalCryptoDir(peer *node2.Peer, cryptoType string) string {
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

func (p *platform) userLocalCryptoDir(peer *node2.Peer, user, cryptoMaterialType string) string {
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

func (p *platform) nextColor() string {
	color := p.colorIndex%14 + 31
	if color > 37 {
		color = color + 90 - 37
	}

	p.colorIndex++
	return fmt.Sprintf("%dm", color)
}

// PeerPortNames returns the list of ports that need to be reserved for a Peer.
func PeerPortNames() []api.PortName {
	return []api.PortName{ListenPort, P2PPort}
}

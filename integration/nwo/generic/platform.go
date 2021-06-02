/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/viper"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/registry"
)

type Builder interface {
	Cryptogen() string
	Build(path string) string
}

type platform struct {
	Registry           *registry.Registry
	Organizations      []*Organization
	Peers              []*Peer
	Builder            Builder
	EventuallyTimeout  time.Duration
	colorIndex         int
	Resolvers          []*Resolver
	ClientAuthRequired bool
}

func NewPlatform(registry *registry.Registry, Builder Builder) *platform {
	p := &platform{
		Registry:          registry,
		Builder:           Builder,
		EventuallyTimeout: 10 * time.Minute,
	}
	p.CheckTopology()
	return p
}

func (p *platform) Name() string {
	return "generic"
}

func (p *platform) GenerateConfigTree() {
	p.GenerateCryptoConfig()
}

func (p *platform) GenerateArtifacts() {
	sess, err := p.Cryptogen(Generate{
		Config: p.CryptoConfigPath(),
		Output: p.CryptoPath(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, p.EventuallyTimeout).Should(gexec.Exit(0))

	p.ConcatenateTLSCACertificates()

	p.GenerateResolverMap()
	for _, peer := range p.Peers {
		cc := &grpc.ConnectionConfig{
			Address:           p.PeerAddressByName(peer, ListenPort),
			TLSEnabled:        true,
			TLSRootCertFile:   path.Join(p.PeerLocalTLSDir(peer), "ca.crt"),
			ConnectionTimeout: 10 * time.Minute,
		}
		p.Registry.ConnectionConfigs[peer.Name] = cc

		clientID, err := p.GetSigningIdentity(peer)
		Expect(err).ToNot(HaveOccurred())
		p.Registry.ClientSigningIdentities[peer.Name] = clientID

		cert, err := ioutil.ReadFile(p.PeerLocalMSPIdentityCert(peer))
		Expect(err).ToNot(HaveOccurred())
		p.Registry.ViewIdentities[peer.Name] = cert

		p.GenerateCoreConfig(peer)
	}
}

func (p *platform) Load() {
	p.CheckTopology()

	for _, peer := range p.Peers {
		v := viper.New()
		v.SetConfigFile(p.NodeConfigPath(peer))
		err := v.ReadInConfig() // Find and read the config file
		Expect(err).NotTo(HaveOccurred())

		cc := &grpc.ConnectionConfig{
			Address:           v.GetString("fsc.address"),
			TLSEnabled:        true,
			TLSRootCertFile:   path.Join(p.PeerLocalTLSDir(peer), "ca.crt"),
			ConnectionTimeout: 10 * time.Minute,
		}
		p.Registry.ConnectionConfigs[peer.Name] = cc

		clientID, err := p.GetSigningIdentity(peer)
		Expect(err).ToNot(HaveOccurred())
		p.Registry.ClientSigningIdentities[peer.Name] = clientID

		cert, err := ioutil.ReadFile(p.PeerLocalMSPIdentityCert(peer))
		Expect(err).ToNot(HaveOccurred())
		p.Registry.ViewIdentities[peer.Name] = cert
	}
}

func (p *platform) Members() []grouper.Member {
	return nil
}

func (p *platform) PostRun() {}

func (p *platform) Cleanup() {}

func (p *platform) CheckTopology() {
	fscTopology := p.Registry.TopologyByName("fsc").(*fsc.Topology)
	orgName := "fsc"

	org := &Organization{
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
	for _, node := range fscTopology.Nodes {
		var extraIdentities []*PeerIdentity
		peer := &Peer{
			Name:            node.Name,
			Organization:    org.Name,
			Type:            FSCNode,
			Bootstrap:       node.Bootstrap,
			ExecutablePath:  node.ExecutablePath,
			ExtraIdentities: extraIdentities,
		}
		p.Peers = append(p.Peers, peer)
		p.Registry.PortsByPeerID[peer.ID()] = p.Registry.PortsByPeerID[node.Name]
		users[orgName] = users[orgName] + 1
		userNames[orgName] = append(userNames[orgName], node.Name)

		po := node.PlatformOpts()
		po.Put("NodeLocalCertPath", p.PeerLocalMSPIdentityCert(peer))
		po.Put("NodeLocalPrivateKeyPath", p.PeerLocalMSPPrivateKey(peer))
		po.Put("NodeLocalTLSDir", p.PeerLocalTLSDir(peer))
	}

	for _, organization := range p.Organizations {
		organization.Users += users[organization.Name]
		organization.UserNames = append(userNames[organization.Name], "User1", "User2")
	}
}

func (p *platform) Cryptogen(command common.Command) (*gexec.Session, error) {
	cmd := common.NewCommand(p.Builder.Cryptogen(), command)
	return p.StartSession(cmd, command.SessionName())
}

func (p *platform) GenerateCryptoConfig() {
	crypto, err := os.Create(p.CryptoConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer crypto.Close()

	t, err := template.New("crypto").Parse(DefaultCryptoTemplate)
	Expect(err).NotTo(HaveOccurred())

	Expect(t.Execute(io.MultiWriter(crypto), p)).NotTo(HaveOccurred())
}

func (p *platform) CryptoPath() string {
	return filepath.Join(p.Registry.RootDir, "crypto")
}

func (p *platform) CryptoConfigPath() string {
	return filepath.Join(p.Registry.RootDir, "crypto-config.yaml")
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

func (p *platform) GenerateCoreConfig(peer *Peer) {
	coreTemplate := DefaultViewExtensionTemplate
	t, err := template.New("peer").Funcs(template.FuncMap{
		"Peer":              func() *Peer { return peer },
		"ToLower":           func(s string) string { return strings.ToLower(s) },
		"ReplaceAll":        func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
		"CACertsBundlePath": func() string { return p.CACertsBundlePath() },
	}).Parse(coreTemplate)
	Expect(err).NotTo(HaveOccurred())

	//pw := gexec.NewPrefixedWriter(fmt.Sprintf("[%s#extension#core.yaml] ", p.ID()), ginkgo.GinkgoWriter)
	extension := bytes.NewBuffer([]byte{})
	err = t.Execute(io.MultiWriter(extension), p)
	p.Registry.AddExtension(peer.Name, registry.GenericExtension, extension.String())
	Expect(err).NotTo(HaveOccurred())
}

func (p *platform) Organization(orgName string) *Organization {
	for _, org := range p.Organizations {
		if org.Name == orgName {
			return org
		}
	}
	return nil
}

func (p *platform) CACertsBundlePath() string {
	return filepath.Join(p.Registry.RootDir, "crypto", "ca-certs.pem")
}

func (p *platform) PeerLocalTLSDir(peer *Peer) string {
	return p.peerLocalCryptoDir(peer, "tls")
}

func (p *platform) PeerLocalMSPIdentityCert(peer *Peer) string {
	return filepath.Join(
		p.peerLocalCryptoDir(peer, "msp"),
		"signcerts",
		peer.Name+"."+p.Organization(peer.Organization).Domain+"-cert.pem",
	)
}

func (p *platform) PeerLocalMSPPrivateKey(peer *Peer) string {
	return filepath.Join(
		p.peerLocalCryptoDir(peer, "msp"),
		"keystore",
		"priv_sk",
	)
}

func (p *platform) PeerOrgs() []*Organization {
	orgsByName := map[string]*Organization{}
	for _, peer := range p.Peers {
		orgsByName[peer.Organization] = p.Organization(peer.Organization)
	}

	var orgs []*Organization
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

func (p *platform) PeersInOrg(orgName string) []*Peer {
	var peers []*Peer
	for _, o := range p.Peers {
		if o.Organization == orgName {
			peers = append(peers, o)
		}
	}
	return peers
}

func (p *platform) PeerAddressByName(peer *Peer, portName registry.PortName) string {
	return fmt.Sprintf("127.0.0.1:%d", p.PeerPortByName(peer, portName))
}

func (p *platform) PeerPortByName(peer *Peer, portName registry.PortName) uint16 {
	peerPorts := p.Registry.PortsByPeerID[peer.Name]
	Expect(peerPorts).NotTo(BeNil())
	return peerPorts[portName]
}

func (p *platform) Peer(orgName, peerName string) *Peer {
	for _, p := range p.PeersInOrg(orgName) {
		if p.Name == peerName {
			return p
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

func (p *platform) listTLSCACertificates() []string {
	fileName2Path := make(map[string]string)
	filepath.Walk(filepath.Join(p.Registry.RootDir, "crypto"), func(path string, info os.FileInfo, err error) error {
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

func (p *platform) peerLocalCryptoDir(peer *Peer, cryptoType string) string {
	org := p.Organization(peer.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		p.Registry.RootDir,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", peer.Name, org.Domain),
		cryptoType,
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

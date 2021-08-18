/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"

	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/helpers"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver/interop"
)

const (
	RelayServerImage   = "hyperledger-labs/weaver-relay-server:latest"
	FabricDriverImager = "hyperledger-labs/weaver-fabric-driver:latest"
)

var RequiredImages = []string{
	RelayServerImage,
	FabricDriverImager,
}

type Builder interface {
	Build(path string) string
}

type FabricNetwork interface {
	DeployChaincode(chaincode *topology.ChannelChaincode)
	DefaultIdemixOrgMSPDir() string
	Topology() *topology.Topology
	PeerChaincodeAddress(peerName string) string
	PeerOrgs() []*fabric.Org
	OrgMSPID(orgName string) string
	PeersByOrg(orgName string) []*fabric.Peer
	UserByOrg(organization string, user string) *fabric.User
	UsersByOrg(organization string) []*fabric.User
	Channels() []*fabric.Channel
	InvokeChaincode(cc *topology.ChannelChaincode, method string, args ...[]byte)
}

type platformFactory struct{}

func NewPlatformFactory() *platformFactory {
	return &platformFactory{}
}

func (f platformFactory) Name() string {
	return TopologyName
}

func (f platformFactory) New(registry api2.Context, t api2.Topology, builder api2.Builder) api2.Platform {
	return NewPlatform(registry, t, builder)
}

type Platform struct {
	Context           api2.Context
	Topology          *Topology
	Builder           api2.Builder
	EventuallyTimeout time.Duration

	NetworkID    string
	DockerClient *docker.Client

	colorIndex int
}

func NewPlatform(ctx api2.Context, t api2.Topology, builder api2.Builder) *Platform {
	helpers.AssertImagesExist(RequiredImages...)

	dockerClient, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())
	networkID := common.UniqueName()
	_, err = dockerClient.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:   networkID,
			Driver: "bridge",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	return &Platform{
		Context:           ctx,
		Topology:          t.(*Topology),
		Builder:           builder,
		EventuallyTimeout: 10 * time.Minute,
		NetworkID:         networkID,
		DockerClient:      dockerClient,
	}
}

func (p *Platform) Name() string {
	return TopologyName
}

func (p *Platform) Type() string {
	return TopologyName
}

func (p *Platform) GenerateConfigTree() {
	for _, relay := range p.Topology.Relays {
		relay.Port = p.Context.ReservePort()
		for _, driver := range relay.Drivers {
			driver.Port = p.Context.ReservePort()
		}
	}
}

func (p *Platform) GenerateArtifacts() {
	for _, relay := range p.Topology.Relays {
		p.generateRelayServerTOML(relay)
		p.generateFabricDriverConfigFiles(relay)
		p.generateInteropChaincodeConfigFiles(relay)
	}
	p.generateFabricExtension()
	p.copyInteropChaincode()
}

func (p *Platform) Load() {
}

func (p *Platform) Members() []grouper.Member {
	return nil
}

func (p *Platform) PostRun() {
	for _, relay := range p.Topology.Relays {
		cc, err := p.PrepareInteropChaincode(relay)
		Expect(err).NotTo(HaveOccurred())

		sampleCC := p.PrepareSampleChaincode(relay)

		fabric := p.Fabric(relay)
		fabric.DeployChaincode(cc)
		fabric.DeployChaincode(sampleCC)

		fabric.InvokeChaincode(sampleCC, "invoke", []byte("alice"), []byte("bob"), []byte("50"))

	}

	for _, relay := range p.Topology.Relays {
		cc, err := p.PrepareInteropChaincode(relay)
		Expect(err).NotTo(HaveOccurred())

		fabric := p.Fabric(relay)

		for _, currentRelay := range p.Topology.Relays {
			if currentRelay.Name == relay.Name {
				continue
			}
			raw, err := ioutil.ReadFile(p.RelayServerInteropAccessControl(currentRelay))
			Expect(err).NotTo(HaveOccurred())
			fabric.InvokeChaincode(cc, "CreateAccessControlPolicy", raw)

			raw, err = ioutil.ReadFile(p.RelayServerInteropVerificationPolicy(currentRelay))
			Expect(err).NotTo(HaveOccurred())
			fabric.InvokeChaincode(cc, "CreateVerificationPolicy", raw)

			raw, err = ioutil.ReadFile(p.RelayServerInteropMembership(currentRelay))
			Expect(err).NotTo(HaveOccurred())
			fabric.InvokeChaincode(cc, "CreateMembership", raw)
		}
	}

	for _, relay := range p.Topology.Relays {
		for _, driver := range relay.Drivers {
			p.RunRelayFabricDriver(
				relay.FabricTopologyName,
				relay.Hostname, strconv.Itoa(int(relay.Port)),
				driver.Hostname, strconv.Itoa(int(driver.Port)),
				relay.InteropChaincode.Label,
				p.FabricDriverConnectionProfilePath(relay),
				p.FabricDriverConfigPath(relay),
				p.FabricDriverWalletDir(relay),
			)
		}

		p.RunRelayServer(
			strings.Replace(relay.Name, "Fabric_", "", -1),
			p.RelayServerConfigPath(relay),
			strconv.Itoa(int(relay.Port)),
		)
	}
}

func (p *Platform) Cleanup() {
	if p.DockerClient == nil {
		return
	}

	cleanupFunc()

	nw, err := p.DockerClient.NetworkInfo(p.NetworkID)
	if _, ok := err.(*docker.NoSuchNetwork); err != nil && ok {
		return
	}
	Expect(err).NotTo(HaveOccurred())

	err = p.DockerClient.RemoveNetwork(nw.ID)
	Expect(err).NotTo(HaveOccurred())

	return

	// containers, err := p.DockerClient.ListContainers(docker.ListContainersOptions{All: true})
	// Expect(err).NotTo(HaveOccurred())
	// for _, c := range containers {
	// 	for _, name := range c.Names {
	// 		if strings.HasPrefix(name, "/"+p.NetworkID) {
	// 			err := p.DockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID, Force: true})
	// 			Expect(err).NotTo(HaveOccurred())
	// 			break
	// 		}
	// 	}
	// }
	//
	// images, err := p.DockerClient.ListImages(docker.ListImagesOptions{All: true})
	// Expect(err).NotTo(HaveOccurred())
	// for _, i := range images {
	// 	for _, tag := range i.RepoTags {
	// 		if strings.HasPrefix(tag, p.NetworkID) {
	// 			err := p.DockerClient.RemoveImage(i.ID)
	// 			Expect(err).NotTo(HaveOccurred())
	// 			break
	// 		}
	// 	}
	// }
}

func (p *Platform) RelayServerDir(relay *RelayServer) string {
	return filepath.Join(
		p.Context.RootDir(),
		"weaver",
		"relay",
		"server",
		relay.Name,
	)
}

func (p *Platform) RelayServerConfigPath(relay *RelayServer) string {
	return filepath.Join(
		p.RelayServerDir(relay),
		"server.toml",
	)
}

func (p *Platform) FabricDriverDir(relay *RelayServer) string {
	return filepath.Join(
		p.Context.RootDir(),
		"weaver",
		"relay",
		"fabric-driver",
		relay.Name,
	)
}

func (p *Platform) FabricDriverConnectionProfilePath(relay *RelayServer) string {
	return filepath.Join(
		p.FabricDriverDir(relay),
		"cp.json",
	)
}

func (p *Platform) FabricDriverConfigPath(relay *RelayServer) string {
	return filepath.Join(
		p.FabricDriverDir(relay),
		"config.json",
	)
}

func (p *Platform) FabricDriverWalletDir(relay *RelayServer) string {
	return filepath.Join(
		p.Context.RootDir(),
		"weaver",
		"relay",
		"fabric-driver",
		relay.Name,
		"wallet-"+relay.FabricTopologyName,
	)
}

func (p *Platform) FabricDriverWalletId(relay *RelayServer, id string) string {
	return filepath.Join(
		p.FabricDriverWalletDir(relay),
		id+".id",
	)
}

func (p *Platform) RelayServerInteropDir(relay *RelayServer) string {
	return filepath.Join(
		p.Context.RootDir(),
		"weaver",
		"relay",
		"server",
		relay.Name,
		"interop-chaincode",
	)
}

func (p *Platform) RelayServerInteropAccessControl(relay *RelayServer) string {
	return filepath.Join(
		p.RelayServerInteropDir(relay),
		"access_control.json",
	)
}

func (p *Platform) RelayServerInteropMembership(relay *RelayServer) string {
	return filepath.Join(
		p.RelayServerInteropDir(relay),
		"membership.json",
	)
}

func (p *Platform) RelayServerInteropVerificationPolicy(relay *RelayServer) string {
	return filepath.Join(
		p.RelayServerInteropDir(relay),
		"verification_policy.json",
	)
}

func (p *Platform) RelayDir() string {
	return filepath.Join(
		p.Context.RootDir(),
		"weaver",
		"relay",
	)
}

func (p *Platform) Fabric(relay *RelayServer) FabricNetwork {
	return p.Context.PlatformByName(relay.FabricTopologyName).(FabricNetwork)
}

func (p *Platform) generateRelayServerTOML(relay *RelayServer) {
	err := os.MkdirAll(p.RelayServerDir(relay), 0o755)
	Expect(err).NotTo(HaveOccurred())

	relayServerFile, err := os.Create(p.RelayServerConfigPath(relay))
	Expect(err).NotTo(HaveOccurred())
	defer relayServerFile.Close()

	var relays []*RelayServer
	for _, r := range p.Topology.Relays {
		if r != relay {
			relays = append(relays, r)
		}
	}

	fmt.Printf("#### %s %s:%d\n", relay.Name, relay.Hostname, relay.Port)

	t, err := template.New("relay_server").Funcs(template.FuncMap{
		"Name":     func() string { return relay.FabricTopologyName },
		"Port":     func() uint16 { return relay.Port },
		"Hostname": func() string { return relay.Hostname },
		"Networks": func() []*Network { return relay.Networks },
		"Drivers":  func() []*Driver { return relay.Drivers },
		"Relays":   func() []*RelayServer { return relays },
	}).Parse(RelayServerTOML)
	Expect(err).NotTo(HaveOccurred())

	err = t.Execute(io.MultiWriter(relayServerFile), p)
	Expect(err).NotTo(HaveOccurred())
}

func (p *Platform) generateFabricDriverConfigFiles(relay *RelayServer) {
	p.generateFabricDriverCPFile(relay)
	p.generateFabricDriverConfigFile(relay)
	p.generateFabricDriverWallet(relay)
}

func (p *Platform) generateFabricDriverCPFile(relay *RelayServer) {
	cp := &ConnectionProfile{
		Name:    relay.Name,
		Version: "1.0.0",
		Client: Client{
			Organization: relay.Organization,
			Connection: Connection{
				Timeout: Timeout{
					Peer: map[string]string{
						"endorser": "300",
					},
				},
			},
		},
		Organizations:          map[string]Organization{},
		Peers:                  map[string]Peer{},
		CertificateAuthorities: map[string]CertificationAuthority{},
	}

	fabric := p.Fabric(relay)
	orgs := fabric.PeerOrgs()
	for _, org := range orgs {
		peers := fabric.PeersByOrg(org.Name)
		var names []string
		for _, peer := range peers {
			names = append(names, peer.FullName)
		}
		cp.Organizations[org.Name] = Organization{
			MSPID: org.MSPID,
			Peers: names,
			CertificateAuthorities: []string{
				"ca." + relay.Name,
			},
		}
		cp.CertificateAuthorities["ca."+relay.Name] = CertificationAuthority{
			Url:        "https://127.0.0.1:7054",
			CaName:     "ca." + relay.Name,
			TLSCACerts: nil,
			HttpOptions: HttpOptions{
				Verify: true,
			},
		}
		for _, peer := range peers {
			_, port, err := net.SplitHostPort(peer.ListeningAddress)
			Expect(err).NotTo(HaveOccurred())

			var certificates []string
			for _, cert := range peer.TLSCACerts {
				raw, err := ioutil.ReadFile(cert)
				Expect(err).NotTo(HaveOccurred())
				certificates = append(certificates, string(raw))
			}
			Expect(len(certificates)).ToNot(BeZero())

			cp.Peers[peer.FullName] = Peer{
				URL: "grpcs://" + net.JoinHostPort(p.localIP(), port),
				TLSCACerts: map[string]string{
					"pem": certificates[0],
				},
				GrpcOptions: map[string]string{
					"ssl-target-name-override": peer.FullName,
					"hostnameOverride":         peer.FullName,
				},
			}
		}

	}

	raw, err := json.MarshalIndent(cp, "", "  ")
	Expect(err).NotTo(HaveOccurred())

	fmt.Println(string(raw))

	Expect(os.MkdirAll(p.FabricDriverDir(relay), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(p.FabricDriverConnectionProfilePath(relay), raw, 0o755)).NotTo(HaveOccurred())
}

func (p *Platform) generateFabricDriverConfigFile(relay *RelayServer) {
	fabric := p.Fabric(relay)
	relayUser := fabric.UserByOrg(relay.Organization, "Relay")
	relayAdmin := fabric.UserByOrg(relay.Organization, "RelayAdmin")
	config := &Config{
		Admin: Admin{
			Name:   relayAdmin.Name,
			Secret: "adminpw",
		},
		Relay: Relay{
			Name:        relayUser.Name,
			Affiliation: "",
			Role:        "client",
			Attrs: []Attr{
				{
					Name:  "relay",
					Value: "true",
					Ecert: true,
				},
			},
		},
		MspId: fabric.OrgMSPID(relay.Organization),
		CaUrl: "",
	}
	raw, err := json.MarshalIndent(config, "", "  ")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(p.FabricDriverDir(relay), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(p.FabricDriverConfigPath(relay), raw, 0o755)).NotTo(HaveOccurred())
}

func (p *Platform) generateFabricDriverWallet(relay *RelayServer) {
	fabric := p.Fabric(relay)

	// User
	relayUser := fabric.UserByOrg(relay.Organization, "Relay")
	cert, err := ioutil.ReadFile(relayUser.Cert)
	Expect(err).NotTo(HaveOccurred())
	key, err := ioutil.ReadFile(relayUser.Key)
	Expect(err).NotTo(HaveOccurred())

	identity := &Identity{
		Credentials: Credentials{
			Certificate: string(cert),
			PrivateKey:  string(key),
		},
		MspId:   fabric.OrgMSPID(relay.Organization),
		Type:    "X.509",
		Version: 1,
	}
	raw, err := json.MarshalIndent(identity, "", "  ")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(p.FabricDriverWalletDir(relay), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(p.FabricDriverWalletId(relay, relayUser.Name), raw, 0o755)).NotTo(HaveOccurred())

	// Admin
	relayAdmin := fabric.UserByOrg(relay.Organization, "RelayAdmin")
	cert, err = ioutil.ReadFile(relayAdmin.Cert)
	Expect(err).NotTo(HaveOccurred())
	key, err = ioutil.ReadFile(relayAdmin.Key)
	Expect(err).NotTo(HaveOccurred())

	identity = &Identity{
		Credentials: Credentials{
			Certificate: string(cert),
			PrivateKey:  string(key),
		},
		MspId:   fabric.OrgMSPID(relay.Organization),
		Type:    "X.509",
		Version: 1,
	}
	raw, err = json.MarshalIndent(identity, "", "  ")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(p.FabricDriverWalletDir(relay), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(p.FabricDriverWalletId(relay, relayAdmin.Name), raw, 0o755)).NotTo(HaveOccurred())
}

func (p *Platform) generateInteropChaincodeConfigFiles(relay *RelayServer) {
	p.generateInteropChaincodeAccessControlFile(relay)
	p.generateInteropChaincodeMembershipFile(relay)
	p.generateInteropChaincodeVerificationPolicyFile(relay)
}

func (p *Platform) generateInteropChaincodeAccessControlFile(relay *RelayServer) {
	fabric := p.Fabric(relay)

	// For all users in all organizations, add a rule per chaincode deployed
	var rules []*interop.Rule
	for _, ch := range fabric.Channels() {
		for _, chaincode := range ch.Chaincodes {
			for _, org := range fabric.PeerOrgs() {
				for _, user := range fabric.UsersByOrg(org.Name) {
					raw, err := ioutil.ReadFile(user.Cert)
					Expect(err).NotTo(HaveOccurred())
					rules = append(rules, &interop.Rule{
						Principal:     string(raw),
						PrincipalType: "ca",
						Resource:      fmt.Sprintf("%s:%s:Read:*", ch.Name, chaincode.Name),
						Read:          false,
					})

				}
			}
		}
	}
	accessControl := &interop.AccessControl{
		SecurityDomain: relay.Name,
		Rules:          rules,
	}
	raw, err := json.MarshalIndent(accessControl, "", "  ")
	Expect(err).ToNot(HaveOccurred())

	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(p.RelayServerInteropDir(relay), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(p.RelayServerInteropAccessControl(relay), raw, 0o755)).NotTo(HaveOccurred())
}

func (p *Platform) generateInteropChaincodeMembershipFile(relay *RelayServer) {
	fabric := p.Fabric(relay)

	// For all users in all organizations, add a rule per chaincode deployed
	members := map[string]*interop.Member{}
	for _, org := range fabric.PeerOrgs() {
		raw, err := ioutil.ReadFile(org.CACertsBundlePat)
		Expect(err).NotTo(HaveOccurred())
		members[org.MSPID] = &interop.Member{
			Type:  "ca",
			Value: string(raw),
		}
	}
	membership := &interop.Membership{
		SecurityDomain: relay.Name,
		Members:        members,
	}
	raw, err := json.MarshalIndent(membership, "", "  ")
	Expect(err).ToNot(HaveOccurred())

	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(p.RelayServerInteropDir(relay), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(p.RelayServerInteropMembership(relay), raw, 0o755)).NotTo(HaveOccurred())
}

func (p *Platform) generateInteropChaincodeVerificationPolicyFile(relay *RelayServer) {
	fabric := p.Fabric(relay)

	// For all users in all organizations, add a rule per chaincode deployed
	var identifiers []*interop.Identifier
	var orgMSPIDs []string
	for _, org := range fabric.PeerOrgs() {
		orgMSPIDs = append(orgMSPIDs, org.MSPID)
	}
	for _, ch := range fabric.Channels() {
		for _, chaincode := range ch.Chaincodes {
			identifiers = append(identifiers, &interop.Identifier{
				Pattern: fmt.Sprintf("%s:%s:query:*", ch.Name, chaincode.Name),
				Policy: &interop.Policy{
					Type:     "Signature",
					Criteria: orgMSPIDs,
				},
			})
		}
	}
	verificationPolicy := &interop.VerificationPolicy{
		SecurityDomain: strings.Replace(relay.Name, "Fabric_", "", -1),
		Identifiers:    identifiers,
	}
	raw, err := json.MarshalIndent(verificationPolicy, "", "  ")
	Expect(err).ToNot(HaveOccurred())

	fmt.Println("GENERATING verification policy:", string(raw))

	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(p.RelayServerInteropDir(relay), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(p.RelayServerInteropVerificationPolicy(relay), raw, 0o755)).NotTo(HaveOccurred())
}

func (p *Platform) generateFabricExtension() {
	fscTopology := p.Context.TopologyByName("fsc").(*fsc.Topology)
	for _, node := range fscTopology.Nodes {
		opt := fabric.Options(&node.Options)

		var servers []*RelayServer
		for _, relay := range p.Topology.Relays {
			for _, organization := range opt.Organizations() {
				if relay.FabricTopologyName == organization.Network {
					servers = append(servers, relay)
					break
				}
			}
		}

		t, err := template.New("view_extension").Funcs(template.FuncMap{
			"Servers": func() []*RelayServer { return servers },
			"RelaysOf": func(relay *RelayServer) []*RelayServer {
				var relays []*RelayServer
				for _, r := range p.Topology.Relays {
					if r != relay {
						relays = append(relays, r)
					}
				}
				return relays
			},
		}).Parse(FabricExtensionTemplate)
		Expect(err).NotTo(HaveOccurred())

		extension := bytes.NewBuffer([]byte{})
		err = t.Execute(io.MultiWriter(extension), p)
		Expect(err).NotTo(HaveOccurred())

		p.Context.AddExtension(node.ID(), api2.FabricExtension, extension.String())
	}
}

func (p *Platform) copyInteropChaincode() {
	src, cleanup, err := packageChaincode()
	Expect(err).ToNot(HaveOccurred())

	defer cleanup()

	dst := p.InteropChaincodeFile()
	sourceFileStat, err := os.Stat(src)
	Expect(err).ToNot(HaveOccurred())

	Expect(sourceFileStat.Mode().IsRegular()).To(BeTrue())
	source, err := os.Open(src)
	Expect(err).ToNot(HaveOccurred())
	defer source.Close()
	destination, err := os.Create(dst)
	Expect(err).ToNot(HaveOccurred())
	defer destination.Close()
	_, err = io.Copy(destination, source)
	Expect(err).ToNot(HaveOccurred())
}

func (p *Platform) localIP() string {
	ni, err := p.DockerClient.NetworkInfo(p.NetworkID)
	Expect(err).NotTo(HaveOccurred())

	Expect(ni.IPAM.Config).To(HaveLen(1))
	var config docker.IPAMConfig
	for _, cfg := range ni.IPAM.Config {
		config = cfg
		break
	}

	dockerPrefix := config.Subnet[:strings.Index(config.Subnet, ".0")]

	ifaces, err := net.Interfaces()
	Expect(err).NotTo(HaveOccurred())

	for _, i := range ifaces {
		addrs, err := i.Addrs()
		Expect(err).NotTo(HaveOccurred())

		for _, addr := range addrs {
			if strings.Index(addr.String(), dockerPrefix) == 0 {
				ipWithSubnet := addr.String()
				i := strings.Index(ipWithSubnet, "/")
				return ipWithSubnet[:i]
			}
		}
	}

	// ginkgo.Fail(fmt.Sprintf("could not find network interface with subnet %s", dockerPrefix))

	return "localhost"
}

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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	fabric2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver/interop"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"
)

const (
	RelayServerImage  = "hyperledger-labs/weaver-relay-server:latest"
	FabricDriverImage = "hyperledger-labs/weaver-fabric-driver:latest"
	// FabricDriverImage = "fabric-driver:latest"
)

var RequiredImages = []string{
	RelayServerImage,
	FabricDriverImage,
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
	PeersByOrg(fabricHost string, orgName string, includeAll bool) []*fabric.Peer
	UserByOrg(organization string, user string) *fabric.User
	UsersByOrg(organization string) []*fabric.User
	Orderers() []*fabric.Orderer
	Channels() []*fabric.Channel
	InvokeChaincode(cc *topology.ChannelChaincode, method string, args ...[]byte) []byte
	ConnectionProfile(name string, ca bool) *network.ConnectionProfile
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

	NetworkID string
}

func NewPlatform(ctx api2.Context, t api2.Topology, builder api2.Builder) *Platform {
	return &Platform{
		Context:           ctx,
		Topology:          t.(*Topology),
		Builder:           builder,
		EventuallyTimeout: 10 * time.Minute,
		NetworkID:         common.UniqueName(),
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

func (p *Platform) PostRun(bool) {
	// getting our docker helper, check required images exists and launch a docker network
	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	err = d.CheckImagesExist(RequiredImages...)
	Expect(err).NotTo(HaveOccurred())

	err = d.CreateNetwork(p.NetworkID)
	Expect(err).NotTo(HaveOccurred())

	for _, relay := range p.Topology.Relays {
		cc, err := p.PrepareInteropChaincode(relay)
		Expect(err).NotTo(HaveOccurred())

		fabric := p.Fabric(relay)
		fabric.DeployChaincode(cc)
	}

	for _, destinationRelay := range p.Topology.Relays {
		cc, err := p.PrepareInteropChaincode(destinationRelay)
		Expect(err).NotTo(HaveOccurred())

		destinationFabric := p.Fabric(destinationRelay)

		for _, sourceRelay := range p.Topology.Relays {
			if sourceRelay.Name == destinationRelay.Name {
				continue
			}
			raw, err := ioutil.ReadFile(p.RelayServerInteropAccessControl(destinationRelay, sourceRelay))
			Expect(err).NotTo(HaveOccurred())
			destinationFabric.InvokeChaincode(cc, "CreateAccessControlPolicy", raw)

			raw, err = ioutil.ReadFile(p.RelayServerInteropVerificationPolicy(sourceRelay))
			Expect(err).NotTo(HaveOccurred())
			destinationFabric.InvokeChaincode(cc, "CreateVerificationPolicy", raw)

			raw, err = ioutil.ReadFile(p.RelayServerInteropMembership(sourceRelay))
			Expect(err).NotTo(HaveOccurred())
			destinationFabric.InvokeChaincode(cc, "CreateMembership", raw)
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

		time.Sleep(time.Second * 2)

		p.RunRelayServer(
			strings.Replace(relay.Name, "Fabric_", "", -1),
			p.RelayServerConfigPath(relay),
			strconv.Itoa(int(relay.Port)),
		)

		time.Sleep(time.Second * 2)
	}
}

func (p *Platform) Cleanup() {
	cleanupFunc()

	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	// remove all weaver related containers
	err = d.Cleanup(p.NetworkID, func(name string) bool {
		return strings.HasPrefix(name, "/driver") || strings.HasPrefix(name, "/relay")
	})
	Expect(err).NotTo(HaveOccurred())
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

func (p *Platform) RelayServerInteropAccessControl(destination *RelayServer, source *RelayServer) string {
	return filepath.Join(
		p.RelayServerInteropDir(destination),
		fmt.Sprintf("access_control_%s.json", source.Name),
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
	cp := p.Fabric(relay).ConnectionProfile(relay.Name, true)
	cp.Client = network.Client{
		Organization: relay.Organization,
		Connection: network.Connection{
			Timeout: network.Timeout{
				Peer: map[string]string{
					"endorser": "300",
				},
			},
		},
	}

	raw, err := json.MarshalIndent(cp, "", "  ")
	Expect(err).NotTo(HaveOccurred())

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

func (p *Platform) generateInteropChaincodeAccessControlFile(destinationRelay *RelayServer) {
	// For all source relay different from destination relay
	// For all users and peers in all organizations in the source network, add a rule per chaincode in the destination network
	destinationFabric := p.Fabric(destinationRelay)
	for _, sourceRelay := range p.Topology.Relays {
		var rules []*interop.Rule
		if sourceRelay != destinationRelay {
			sourceFabric := p.Fabric(sourceRelay)

			for _, ch := range destinationFabric.Channels() {
				for _, chaincode := range ch.Chaincodes {

					for _, org := range sourceFabric.PeerOrgs() {

						for _, peer := range sourceFabric.PeersByOrg("", org.Name, true) {
							raw, err := ioutil.ReadFile(peer.Cert)
							Expect(err).NotTo(HaveOccurred())

							rules = append(rules, &interop.Rule{
								Principal:     string(raw),
								PrincipalType: "certificate",
								Resource:      fmt.Sprintf("%s:%s:*", ch.Name, chaincode.Name),
								Read:          false,
							})
						}

						for _, user := range sourceFabric.UsersByOrg(org.Name) {
							raw, err := ioutil.ReadFile(user.Cert)
							Expect(err).NotTo(HaveOccurred())

							rules = append(rules, &interop.Rule{
								Principal:     string(raw),
								PrincipalType: "certificate",
								Resource:      fmt.Sprintf("%s:%s:*", ch.Name, chaincode.Name),
								Read:          false,
							})
						}
					}
				}
			}
		}
		accessControl := &interop.AccessControl{
			SecurityDomain: sourceRelay.Name,
			Rules:          rules,
		}
		raw, err := json.MarshalIndent(accessControl, "", "  ")
		Expect(err).ToNot(HaveOccurred())

		Expect(err).NotTo(HaveOccurred())
		Expect(os.MkdirAll(p.RelayServerInteropDir(destinationRelay), 0o755)).NotTo(HaveOccurred())
		Expect(ioutil.WriteFile(p.RelayServerInteropAccessControl(destinationRelay, sourceRelay), raw, 0o755)).NotTo(HaveOccurred())
	}
}

func (p *Platform) generateInteropChaincodeMembershipFile(relay *RelayServer) {
	fabric := p.Fabric(relay)

	// For all users in all organizations, add a rule per chaincode deployed
	members := map[string]*interop.Member{}
	for _, org := range fabric.PeerOrgs() {
		raw, err := ioutil.ReadFile(org.PeerCACertificatePath)
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

func (p *Platform) generateInteropChaincodeVerificationPolicyFile(destinationRelay *RelayServer) {
	destinationFabric := p.Fabric(destinationRelay)

	// For all source networks

	// For all users in all organizations, add a rule per chaincode deployed
	var identifiers []*interop.Identifier
	for _, ch := range destinationFabric.Channels() {
		for _, chaincode := range ch.Chaincodes {
			identifiers = append(identifiers, &interop.Identifier{
				Pattern: fmt.Sprintf("%s:%s:*", ch.Name, chaincode.Name),
				Policy: &interop.Policy{
					Type:     "Signature",
					Criteria: chaincode.OrgMSPIDs,
				},
			})
		}
	}
	verificationPolicy := &interop.VerificationPolicy{
		SecurityDomain: strings.Replace(destinationRelay.Name, "Fabric_", "", -1),
		Identifiers:    identifiers,
	}
	raw, err := json.MarshalIndent(verificationPolicy, "", "  ")
	Expect(err).ToNot(HaveOccurred())

	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(p.RelayServerInteropDir(destinationRelay), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(p.RelayServerInteropVerificationPolicy(destinationRelay), raw, 0o755)).NotTo(HaveOccurred())
}

func (p *Platform) generateFabricExtension() {
	fscTopology := p.Context.TopologyByName("fsc").(*fsc.Topology)
	for _, node := range fscTopology.Nodes {
		opt := fabric2.Options(node.Options)

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

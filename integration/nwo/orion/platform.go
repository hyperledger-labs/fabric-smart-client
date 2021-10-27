/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger-labs/orion-server/config"
	"github.com/hyperledger-labs/orion-server/pkg/server/testutils"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"
	"gopkg.in/yaml.v2"

	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/helpers"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const (
	ServerImage = "orionbcdb/orion-server"
)

var (
	logger         = flogging.MustGetLogger("nwo.orion")
	RequiredImages = []string{
		ServerImage,
	}
)

type Identity struct {
	Name string
	Cert string
	Key  string
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
	PeersByOrg(orgName string, includeAll bool) []*fabric.Peer
	UserByOrg(organization string, user string) *fabric.User
	UsersByOrg(organization string) []*fabric.User
	Orderers() []*fabric.Orderer
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

	colorIndex  int
	nodePort    uint16
	peerPort    uint16
	localConfig *config.LocalConfiguration
	serverUrl   *url.URL
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
	return p.Topology.TopologyName
}

func (p *Platform) Type() string {
	return TopologyName
}

func (p *Platform) GenerateConfigTree() {
}

func (p *Platform) GenerateArtifacts() {
	for _, subdir := range []string{"crypto", "database", "config", "txs", "wal", "snap"} {
		Expect(os.MkdirAll(filepath.Join(p.rootDir(), subdir), 0o755)).NotTo(HaveOccurred())
	}

	p.writeConfigFile()

	roles := []string{"admin", "server"}
	for _, db := range p.Topology.DBs {
		for _, role := range db.Roles {
			found := false
			for _, r := range roles {
				if r == role {
					found = true
					break
				}
			}
			if !found {
				roles = append(roles, role)
			}
		}
	}

	// fscTopology := p.Context.TopologyByName("fsc").(*fsc.Topology)
	// for _, node := range fscTopology.Nodes {
	// 	opt := Options(&node.Options)
	// 	if len(opt.Role()) != 0 {
	// 		found := false
	// 		for _, r := range roles {
	// 			if r == role {
	// 				found = true
	// 				break
	// 			}
	// 		}
	// 		if found {
	// 			roles = append(roles, role)
	// 		}
	// 	}
	// }
	for _, role := range roles {
		Expect(os.MkdirAll(p.roleCryptoDir(role), 0o755)).NotTo(HaveOccurred())
	}

	// CA
	Expect(os.MkdirAll(p.roleCryptoDir("CA"), 0o755)).NotTo(HaveOccurred())
	cryptoDir := p.cryptoDir()
	rootCAPemCert, caPrivKey, err := testutils.GenerateRootCA("RootCA", "127.0.0.1")
	Expect(err).NotTo(HaveOccurred())

	rootCACertFile, err := os.Create(path.Join(cryptoDir, "CA", "CA.pem"))
	Expect(err).ToNot(HaveOccurred())
	_, err = rootCACertFile.Write(rootCAPemCert)
	Expect(err).NotTo(HaveOccurred())
	rootCACertFile.Close()
	rootCAKeyFile, err := os.Create(path.Join(cryptoDir, "CA", "CA.key"))
	Expect(err).NotTo(HaveOccurred())
	_, err = rootCAKeyFile.Write(rootCAPemCert)
	Expect(err).ToNot(HaveOccurred())
	rootCAKeyFile.Close()

	// Roles
	for _, name := range roles {
		keyPair, err := tls.X509KeyPair(rootCAPemCert, caPrivKey)
		Expect(err).NotTo(HaveOccurred())

		pemCert, privKey, err := testutils.IssueCertificate("Client "+name, "127.0.0.1", keyPair)
		Expect(err).NotTo(HaveOccurred())

		pemCertFile, err := os.Create(path.Join(cryptoDir, name, name+".pem"))
		Expect(err).NotTo(HaveOccurred())
		_, err = pemCertFile.Write(pemCert)
		Expect(err).NotTo(HaveOccurred())
		err = pemCertFile.Close()
		Expect(err).NotTo(HaveOccurred())

		pemPrivKeyFile, err := os.Create(path.Join(cryptoDir, name, name+".key"))
		Expect(err).NotTo(HaveOccurred())
		_, err = pemPrivKeyFile.Write(privKey)
		Expect(err).NotTo(HaveOccurred())
		err = pemPrivKeyFile.Close()
		Expect(err).NotTo(HaveOccurred())
	}

	p.generateExtension()
}

func (p *Platform) Load() {
}

func (p *Platform) Members() []grouper.Member {
	return nil
}

func (p *Platform) PostRun(load bool) {
	p.RunOrionServer()
	p.InitOrionServer()
}

func (p *Platform) Cleanup() {
	if p.DockerClient == nil {
		return
	}

	nw, err := p.DockerClient.NetworkInfo(p.NetworkID)
	if _, ok := err.(*docker.NoSuchNetwork); err != nil && ok {
		return
	}
	Expect(err).NotTo(HaveOccurred())

	containers, err := p.DockerClient.ListContainers(docker.ListContainersOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, c := range containers {
		for _, name := range c.Names {
			p.DockerClient.DisconnectNetwork(p.NetworkID, docker.NetworkConnectionOptions{
				Force:     true,
				Container: c.ID,
			})
			if strings.HasPrefix(name, "/orion") {
				err := p.DockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID, Force: true})
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}

	images, err := p.DockerClient.ListImages(docker.ListImagesOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, i := range images {
		for _, tag := range i.RepoTags {
			if strings.HasPrefix(tag, p.NetworkID) {
				err := p.DockerClient.RemoveImage(i.ID)
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}

	err = p.DockerClient.RemoveNetwork(nw.ID)
	Expect(err).NotTo(HaveOccurred())
}

func (p *Platform) RunOrionServer() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	net, err := p.DockerClient.NetworkInfo(p.NetworkID)
	if err != nil {
		panic(err)
	}

	strNodePort := strconv.Itoa(int(p.nodePort))
	strPeerPort := strconv.Itoa(int(p.peerPort))

	hostname := "orion-" + p.Topology.TopologyName
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: hostname,
		Image:    ServerImage,
		Tty:      false,
		ExposedPorts: nat.PortSet{
			nat.Port(strNodePort + "/tcp"): struct{}{},
			nat.Port(strPeerPort + "/tcp"): struct{}{},
		},
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type: mount.TypeBind,
				// Absolute path to
				Source: p.rootDir(),
				Target: "/etc/orion-server",
			},
		},
		PortBindings: nat.PortMap{
			nat.Port(strNodePort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: strNodePort,
				},
			},
			nat.Port(strPeerPort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: strPeerPort,
				},
			},
		},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			p.NetworkID: {
				NetworkID: net.ID,
			},
		},
	}, nil, hostname)
	if err != nil {
		panic(err)
	}

	cli.NetworkConnect(context.Background(), p.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: p.NetworkID,
	})

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	time.Sleep(60 * time.Second)
}

func (p *Platform) replaceForDocker(origin string) string {
	return strings.Replace(origin, p.rootDir(), "/etc/orion-server", 1)
}

func (p *Platform) generateExtension() {
	fscTopology := p.Context.TopologyByName("fsc").(*fsc.Topology)
	for _, node := range fscTopology.Nodes {
		opt := Options(&node.Options)
		role := opt.Role()

		t, err := template.New("view_extension").Funcs(template.FuncMap{
			"Name":       func() string { return p.Name() },
			"ServerURL":  func() string { return p.serverUrl.String() },
			"ServerID":   func() string { return p.localConfig.Server.Identity.ID },
			"CACertPath": func() string { return p.caPem() },
			"Identities": func() []Identity {
				return []Identity{
					{
						Name: role,
						Cert: p.pem(role),
						Key:  p.key(role),
					},
				}
			},
		}).Parse(ExtensionTemplate)
		Expect(err).NotTo(HaveOccurred())

		extension := bytes.NewBuffer([]byte{})
		err = t.Execute(io.MultiWriter(extension), p)
		Expect(err).NotTo(HaveOccurred())

		p.Context.AddExtension(node.ID(), api2.OrionExtension, extension.String())
	}
}

func (p *Platform) writeConfigFile() {
	p.nodePort = p.Context.ReservePort()
	p.peerPort = p.Context.ReservePort()

	p.localConfig = &config.LocalConfiguration{
		Server: config.ServerConf{
			Identity: config.IdentityConf{
				ID:              "bdb-node-1",
				CertificatePath: p.replaceForDocker(p.serverPem()),
				KeyPath:         p.replaceForDocker(p.serverKey()),
			},
			Network: config.NetworkConf{
				Address: "0.0.0.0",
				Port:    uint32(p.nodePort),
			},
			Database: config.DatabaseConf{
				Name:            "leveldb",
				LedgerDirectory: p.replaceForDocker(p.databaseDir()),
			},
			QueueLength: config.QueueLengthConf{
				Transaction:               10,
				ReorderedTransactionBatch: 10,
				Block:                     10,
			},
			LogLevel: "debug",
		},
		BlockCreation: config.BlockCreationConf{
			MaxBlockSize:                1000000,
			MaxTransactionCountPerBlock: 1,
			BlockTimeout:                500 * time.Millisecond,
		},
		Replication: config.ReplicationConf{
			SnapDir: p.replaceForDocker(p.snapDir()),
			WALDir:  p.replaceForDocker(p.walDir()),
			Network: config.NetworkConf{
				Address: "0.0.0.0",
				Port:    uint32(p.peerPort),
			},
			TLS: config.TLSConf{Enabled: false},
		},
		Bootstrap: config.BootstrapConf{
			Method: "genesis",
			File:   p.replaceForDocker(p.boostrapSharedConfig()),
		},
	}
	bootstrap := &config.SharedConfiguration{
		Nodes: []config.NodeConf{
			{
				NodeID:          "bdb-node-1",
				Host:            "0.0.0.0",
				Port:            uint32(p.nodePort),
				CertificatePath: p.replaceForDocker(p.serverPem()),
			},
		},
		Consensus: &config.ConsensusConf{
			Algorithm: "raft",
			Members: []*config.PeerConf{
				{
					NodeId:   "bdb-node-1",
					RaftId:   1,
					PeerHost: "0.0.0.0",
					PeerPort: uint32(p.peerPort),
				},
			},
			RaftConfig: &config.RaftConf{
				TickInterval:         "100ms",
				ElectionTicks:        100,
				HeartbeatTicks:       10,
				MaxInflightBlocks:    50,
				SnapshotIntervalSize: 1000000000000,
			},
		},
		CAConfig: config.CAConfiguration{
			RootCACertsPath: []string{p.replaceForDocker(p.caPem())},
		},
		Admin: config.AdminConf{
			ID:              "admin",
			CertificatePath: p.replaceForDocker(p.adminPem()),
		},
	}

	c, err := yaml.Marshal(p.localConfig)
	Expect(err).ToNot(HaveOccurred())

	err = ioutil.WriteFile(p.configFile(), c, 0644)
	Expect(err).ToNot(HaveOccurred())

	b, err := yaml.Marshal(bootstrap)
	Expect(err).ToNot(HaveOccurred())

	err = ioutil.WriteFile(p.boostrapSharedConfig(), b, 0644)
	Expect(err).ToNot(HaveOccurred())

	p.serverUrl, err = url.Parse(fmt.Sprintf("http://%s:%d", "127.0.0.1", p.localConfig.Server.Network.Port))
	Expect(err).ToNot(HaveOccurred())

	p.saveServerUrl(p.serverUrl)
	Expect(err).ToNot(HaveOccurred())
}

func (p *Platform) rootDir() string {
	return filepath.Join(
		p.Context.RootDir(),
		"orion",
		p.Topology.TopologyName,
	)
}

func (p *Platform) cryptoDir() string {
	return filepath.Join(
		p.rootDir(),
		"crypto",
	)
}

func (p *Platform) roleCryptoDir(user string) string {
	return filepath.Join(
		p.cryptoDir(),
		user,
	)
}

func (p *Platform) databaseDir() string {
	return filepath.Join(
		p.Context.RootDir(),
		"orion",
		p.Topology.TopologyName,
		"database",
	)
}

func (p *Platform) snapDir() string {
	return filepath.Join(
		p.Context.RootDir(),
		"orion",
		p.Topology.TopologyName,
		"snap",
	)
}

func (p *Platform) walDir() string {
	return filepath.Join(
		p.Context.RootDir(),
		"orion",
		p.Topology.TopologyName,
		"wal",
	)
}

func (p *Platform) serverPem() string {
	return filepath.Join(p.roleCryptoDir("server"), "server.pem")
}

func (p *Platform) serverKey() string {
	return filepath.Join(p.roleCryptoDir("server"), "server.key")
}

func (p *Platform) caPem() string {
	return filepath.Join(p.roleCryptoDir("CA"), "CA.pem")
}

func (p *Platform) adminPem() string {
	return filepath.Join(p.roleCryptoDir("admin"), "admin.pem")
}

func (p *Platform) pem(role string) string {
	return filepath.Join(p.roleCryptoDir(role), role+".pem")
}

func (p *Platform) key(role string) string {
	return filepath.Join(p.roleCryptoDir(role), role+".key")
}

func (p *Platform) configDir() string {
	return filepath.Join(
		p.rootDir(),
		"config",
	)
}

func (p *Platform) boostrapSharedConfig() string {
	return filepath.Join(
		p.configDir(),
		"bootstrap-shared-config.yaml",
	)
}

func (p *Platform) configFile() string {
	return filepath.Join(
		p.configDir(),
		"config.yml",
	)
}

func (p *Platform) saveServerUrl(url *url.URL) {
	serverUrlFile, err := os.Create(filepath.Join(p.rootDir(), "server.url"))
	Expect(err).ToNot(HaveOccurred())
	_, err = serverUrlFile.WriteString(url.String())
	Expect(err).ToNot(HaveOccurred())
	serverUrlFile.Close()
}

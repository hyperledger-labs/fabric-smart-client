/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/orion-server/config"
	"github.com/hyperledger-labs/orion-server/pkg/server/testutils"
	"github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"
	"gopkg.in/yaml.v2"
)

const (
	Port          = network.HostPort
	PeerPortID    = "peer"
	OrdererPortID = "orderer"
)

var logger = logging.MustGetLogger("nwo.orion")

type Identity struct {
	Name string
	Cert string
	Key  string
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

	nodePort    uint16
	peerPort    uint16
	localConfig *config.LocalConfiguration
	serverUrl   *url.URL
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
	return p.Topology.TopologyName
}

func (p *Platform) Type() string {
	return TopologyName
}

func (p *Platform) GenerateConfigTree() {
}

func (p *Platform) GenerateArtifacts() {
	for _, subdir := range []string{"crypto", "database", "config", "ledger", "txs", "wal", "snap"} {
		gomega.Expect(os.MkdirAll(filepath.Join(p.rootDir(), subdir), 0o755)).NotTo(gomega.HaveOccurred())
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
		gomega.Expect(os.MkdirAll(p.roleCryptoDir(role), 0o755)).NotTo(gomega.HaveOccurred())
	}

	// CA
	gomega.Expect(os.MkdirAll(p.roleCryptoDir("CA"), 0o755)).NotTo(gomega.HaveOccurred())
	cryptoDir := p.cryptoDir()
	rootCAPemCert, caPrivKey, err := testutils.GenerateRootCA("RootCA", "127.0.0.1")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	rootCACertFile, err := os.Create(path.Join(cryptoDir, "CA", "CA.pem"))
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	_, err = rootCACertFile.Write(rootCAPemCert)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	utils.IgnoreErrorFunc(rootCACertFile.Close)
	rootCAKeyFile, err := os.Create(path.Join(cryptoDir, "CA", "CA.key"))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	_, err = rootCAKeyFile.Write(rootCAPemCert)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	utils.IgnoreErrorFunc(rootCAKeyFile.Close)

	// Roles
	for _, name := range roles {
		keyPair, err := tls.X509KeyPair(rootCAPemCert, caPrivKey)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		pemCert, privKey, err := testutils.IssueCertificate("Client "+name, "127.0.0.1", keyPair)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		pemCertFile, err := os.Create(path.Join(cryptoDir, name, name+".pem"))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = pemCertFile.Write(pemCert)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = pemCertFile.Close()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		pemPrivKeyFile, err := os.Create(path.Join(cryptoDir, name, name+".key"))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		_, err = pemPrivKeyFile.Write(privKey)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = pemPrivKeyFile.Close()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	p.generateExtension()
}

func (p *Platform) Load() {
}

func (p *Platform) Members() []grouper.Member {
	return nil
}

func (p *Platform) PostRun(load bool) {

	// getting our docker helper, check required images exists and launch a docker network
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = d.CheckImagesExist(RequiredImages...)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = d.CreateNetwork(p.NetworkID)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	p.StartOrionServer()
	err = p.InitOrionServer()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (p *Platform) Cleanup() {
	dockerClient, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = dockerClient.Cleanup(p.NetworkID, func(name string) bool {
		return strings.HasPrefix(name, "/"+p.NetworkID)
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (p *Platform) DeleteVault(id string) {
	fscTopology := p.Context.TopologyByName("fsc").(*fsc.Topology)
	found := false
	for _, node := range fscTopology.Nodes {
		if strings.Contains(node.Name, id) {
			for _, uniqueName := range node.ReplicaUniqueNames() {
				gomega.Expect(os.RemoveAll(fsc.SqlitePath(p.NodeStorages(uniqueName), network.VaultPersistenceKey))).ToNot(gomega.HaveOccurred())
			}
			found = true
		}
	}
	gomega.Expect(found).To(gomega.BeTrue(), "cannot find node [%s]", id)
}

func (p *Platform) replaceForDocker(origin string) string {
	return strings.Replace(origin, p.rootDir(), "/etc/orion-server", 1)
}

func ReadHelperConfig(configPath string) (*HelperConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var c HelperConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, err
}

func (p *Platform) InitOrionServer() error {
	c, err := ReadHelperConfig(p.HelperConfigPath())
	if err != nil {
		return err
	}
	return c.Init()
}

func (p *Platform) generateExtension() {
	fscTopology := p.Context.TopologyByName("fsc").(*fsc.Topology)
	for _, node := range fscTopology.Nodes {
		opt := Options(node.Options)
		role := opt.Role()
		if len(role) == 0 {
			// skip
			continue
		}

		persistenceNames := fsc.GetPersistenceNames(node.Options, network.VaultPersistenceKey)

		for _, uniqueName := range node.ReplicaUniqueNames() {
			t, err := template.New("view_extension").Funcs(template.FuncMap{
				"Name":       func() string { return p.Name() },
				"ServerURL":  func() string { return p.serverUrl.String() },
				"ServerID":   func() string { return p.localConfig.Server.Identity.ID },
				"CACertPath": func() string { return p.caPem() },
				"Identities": func() []Identity {
					return []Identity{
						{
							Name: role,
							Cert: p.PemPath(role),
							Key:  p.KeyPath(role),
						},
					}
				},
				"VaultPersistence": func() driver2.PersistenceName { return persistenceNames[network.VaultPersistenceKey] },
			}).Parse(ExtensionTemplate)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			extension := bytes.NewBuffer([]byte{})
			err = t.Execute(io.MultiWriter(extension), p)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			p.Context.AddExtension(uniqueName, api2.OrionExtension, extension.String())
		}
	}
}

func (p *Platform) writeConfigFile() {
	p.Context.SetPortsByPeerID("", PeerPortID, api2.Ports{Port: p.Context.ReservePort()})
	p.Context.SetPortsByOrdererID("", OrdererPortID, api2.Ports{Port: p.Context.ReservePort()})

	p.nodePort = p.Context.PortsByOrdererID("", OrdererPortID)[Port]
	p.peerPort = p.Context.PortsByPeerID("", PeerPortID)[Port]

	nodeHost := utils.DefaultString(p.Context.HostByOrdererID("", OrdererPortID), "127.0.0.1")
	peerHost := utils.DefaultString(p.Context.HostByPeerID("", PeerPortID), "127.0.0.1")

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
				Transaction:               1000,
				ReorderedTransactionBatch: 1000,
				Block:                     1000,
			},
			LogLevel: "debug",
		},
		BlockCreation: config.BlockCreationConf{
			MaxBlockSize:                1000000,
			MaxTransactionCountPerBlock: 100,
			BlockTimeout:                100 * time.Millisecond,
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
		Nodes: []*config.NodeConf{
			{
				NodeID:          "bdb-node-1",
				Host:            nodeHost,
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
					PeerHost: peerHost,
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
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = os.WriteFile(p.configFile(), c, 0644)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	b, err := yaml.Marshal(bootstrap)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = os.WriteFile(p.boostrapSharedConfig(), b, 0644)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	p.serverUrl, err = url.Parse(fmt.Sprintf("http://%s:%d", nodeHost, p.nodePort))
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	p.saveServerUrl(p.serverUrl)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	certPaths := map[string]string{}
	for _, db := range p.Topology.DBs {
		for _, role := range db.Roles {
			certPaths[role] = p.PemPath(role)
		}
	}
	init := &InitConfig{
		ServerUrl:           p.ServerUrl(),
		CACertPath:          p.caPem(),
		ServerID:            p.ServerID(),
		AdminID:             "admin",
		AdminCertPath:       p.PemPath("admin"),
		AdminPrivateKeyPath: p.KeyPath("admin"),
		DBs:                 p.Topology.DBs,
		CertPaths:           certPaths,
	}

	i, err := yaml.Marshal(HelperConfig{InitConfig: init})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	gomega.Expect(os.MkdirAll(p.configDir(), 0766)).To(gomega.Succeed())
	gomega.Expect(os.WriteFile(p.HelperConfigPath(), i, 0766)).To(gomega.Succeed())
}

func (p *Platform) ServerUrl() string {
	return p.serverUrl.String()
}

func (p *Platform) ServerID() string {
	return p.localConfig.Server.Identity.ID
}

func (p *Platform) rootDir() string {
	return filepath.Join(
		p.Context.RootDir(),
		"orion",
		p.Topology.TopologyName,
	)
}

func (p *Platform) ledgerDir() string {
	return filepath.Join(
		p.rootDir(),
		"ledger",
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

func (p *Platform) serverPem() string { return p.PemPath("server") }

func (p *Platform) serverKey() string { return p.KeyPath("server") }

func (p *Platform) caPem() string { return p.PemPath("CA") }

func (p *Platform) adminPem() string { return p.PemPath("admin") }

func (p *Platform) PemPath(role string) string {
	return filepath.Join(p.roleCryptoDir(role), role+".pem")
}

func (p *Platform) KeyPath(role string) string {
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

func (p *Platform) HelperConfigPath() string {
	return filepath.Join(
		p.configDir(),
		"helper-config.yaml",
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
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	_, err = serverUrlFile.WriteString(url.String())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	utils.IgnoreErrorFunc(serverUrlFile.Close)
}

func (p *Platform) NodeStorages(uniqueName string) string {
	return filepath.Join(p.Context.RootDir(), "fsc", "nodes", uniqueName)
}

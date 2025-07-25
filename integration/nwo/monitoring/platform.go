/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/hle"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/optl"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/onsi/gomega"
	prom_api "github.com/prometheus/client_golang/api"
	prom_v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/tedsuo/ifrit/grouper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var logger = logging.MustGetLogger()

const (
	TopologyName = "monitoring"
)

type platformFactory struct{}

func NewPlatformFactory() *platformFactory {
	return &platformFactory{}
}

func (f platformFactory) Name() string {
	return TopologyName
}

func (f platformFactory) New(registry api.Context, t api.Topology, builder api.Builder) api.Platform {
	return New(registry, t.(*Topology))
}

type Extension interface {
	CheckTopology()
	GenerateArtifacts()
	PostRun(load bool)
}

type Platform struct {
	Context    api.Context
	topology   *Topology
	RootDir    string
	Prefix     string
	Extensions []Extension
	networkID  string
	promAPI    prom_v1.API
	jaegerAPI  api_v2.QueryServiceClient
}

func New(reg api.Context, topology *Topology) *Platform {
	promClient, err := prom_api.NewClient(prom_api.Config{Address: fmt.Sprintf("http://0.0.0.0:%d", topology.PrometheusPort)})
	if err != nil {
		panic(err)
	}
	jaegerClientConn, err := grpc.NewClient(fmt.Sprintf("0.0.0.0:%d", topology.JaegerQueryPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	p := &Platform{
		Context:    reg,
		RootDir:    reg.RootDir(),
		Prefix:     topology.Name(),
		topology:   topology,
		Extensions: []Extension{},
		networkID:  common.UniqueName(),
		promAPI:    prom_v1.NewAPI(promClient),
		jaegerAPI:  api_v2.NewQueryServiceClient(jaegerClientConn),
	}
	p.AddExtension(hle.NewExtension(p))
	p.AddExtension(monitoring.NewExtension(p))
	p.AddExtension(optl.NewExtension(p))

	return p
}

func (p *Platform) Name() string {
	return p.topology.Name()
}

func (p *Platform) Type() string {
	return p.topology.Type()
}

func (p *Platform) GenerateConfigTree() {
}

func (p *Platform) GenerateArtifacts() {
	for _, extension := range p.Extensions {
		extension.CheckTopology()
		extension.GenerateArtifacts()
	}
}

func (p *Platform) Load() {
}

func (p *Platform) Members() []grouper.Member {
	return nil
}

func (p *Platform) PostRun(load bool) {
	logger.Infof("Post execution [%s]...", p.Prefix)

	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// create a container network used for our monitoring services
	err = d.CreateNetwork(p.networkID)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Extensions
	for _, extension := range p.Extensions {
		extension.PostRun(load)
	}

	// Wait a few second to let Fabric stabilize
	time.Sleep(5 * time.Second)
	logger.Infof("Post execution [%s]...done.", p.Prefix)
}

func (p *Platform) Cleanup() {
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// cleanup all monitoring services associated with our network ID
	err = d.Cleanup(p.networkID, func(name string) bool {
		return strings.HasPrefix(name, "/"+p.networkID)
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (p *Platform) PrometheusAPI() prom_v1.API {
	return p.promAPI
}

func (p *Platform) JaegerAPI() api_v2.QueryServiceClient {
	return p.jaegerAPI
}

func (p *Platform) AddExtension(ex Extension) {
	p.Extensions = append(p.Extensions, ex)
}

func (p *Platform) GetContext() api.Context {
	return p.Context
}

func (p *Platform) NetworkID() string {
	return p.networkID
}

func (p *Platform) ConfigDir() string {
	return filepath.Join(p.RootDir, "monitoring")
}

func (p *Platform) HyperledgerExplorer() bool {
	return p.topology.HyperledgerExplorer
}

func (p *Platform) HyperledgerExplorerPort() int {
	return p.topology.HyperledgerExplorerPort
}

func (p *Platform) PrometheusGrafana() bool {
	return p.topology.PrometheusGrafana
}

func (p *Platform) PrometheusPort() int {
	return p.topology.PrometheusPort
}

func (p *Platform) GrafanaPort() int {
	return p.topology.GrafanaPort
}

func (p *Platform) OPTL() bool {
	return p.topology.OPTL
}

func (p *Platform) OPTLPort() int {
	return p.topology.OPTLPort
}

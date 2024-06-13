/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	. "github.com/onsi/gomega"
)

var NoReplication = &ReplicationOptions{}

type ReplicationOptions struct {
	ReplicationFactors map[string]int
	SQLConfigs         map[string]*postgres.PostgresConfig
}

func (o *ReplicationOptions) For(name string) []node.Option {
	opts := make([]node.Option, 0, 3)
	if f := o.ReplicationFactors[name]; f > 0 {
		opts = append(opts, fsc.WithReplicationFactor(f))
	}
	if sqlConfig, ok := o.SQLConfigs[name]; ok {
		opts = append(opts, fsc.WithPostgresPersistence(sqlConfig), fabric.WithPostgresVaultPersistence(sqlConfig))
	}
	return opts
}

func NewTestSuite(generator func() (*Infrastructure, error)) *TestSuite {
	return NewTestSuiteWithSQL(nil, generator)
}

func NewTestSuiteWithSQL(sqlConfigs map[string]*postgres.PostgresConfig, generator func() (*Infrastructure, error)) *TestSuite {
	return &TestSuite{
		sqlConfigs: sqlConfigs,
		generator:  generator,
		closeFunc:  func() {},
	}
}

type TestSuite struct {
	sqlConfigs map[string]*postgres.PostgresConfig
	generator  func() (*Infrastructure, error)

	closeFunc func()
	II        *Infrastructure
}

func (s *TestSuite) TearDown() {
	s.II.Stop()
	s.closeFunc()
}

func (s *TestSuite) Setup() {
	logger.Warnf("setting up for: %v", s.sqlConfigs)
	if len(s.sqlConfigs) > 0 {
		closeFunc, err := postgres.StartPostgresWithFmt(s.sqlConfigs)
		Expect(err).NotTo(HaveOccurred())
		s.closeFunc = closeFunc
	}

	// Create the integration ii
	ii, err := s.generator()
	Expect(err).NotTo(HaveOccurred())
	s.II = ii
	// Start the integration ii
	ii.Start()
	// Sleep for a while to allow the networks to be ready
	time.Sleep(20 * time.Second)
}

func ReplaceTemplate(topologies []api.Topology) []api.Topology {
	for _, topology := range topologies {
		if topology.Type() == fsc.TopologyName {
			topology.(*fsc.Topology).Templates.Node = strings.Replace(node.DefaultTemplate, "fscnode.New()", "fscnode.NewEmpty(\"\")", 1)
		}
	}
	return topologies
}

/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/onsi/gomega"
)

var NoReplication = &ReplicationOptions{}

type ReplicationOptions struct {
	ReplicationFactors map[string]int
	SQLConfigs         map[string]*postgres.ContainerConfig
}

func (o *ReplicationOptions) For(name string) []node.Option {
	opts := make([]node.Option, 0, 3)
	if f := o.ReplicationFactors[name]; f > 0 {
		opts = append(opts, fsc.WithReplicationFactor(f))
	}
	if sqlConfig, ok := o.SQLConfigs[name]; ok {
		opts = append(opts, fabric.WithDefaultPostgresPersistence(sqlConfig),
			fabric.WithPostgresPersistenceNames(common.DefaultPersistence, fabric.VaultPersistencePrefix),
		)
	} else {
		opts = append(opts, fabric.WithSqlitePersistences(fabric.VaultPersistencePrefix))
	}
	return opts
}

func NewTestSuite(generator func() (*Infrastructure, error)) *TestSuite {
	return &TestSuite{
		generator: generator,
	}
}

type TestSuite struct {
	generator func() (*Infrastructure, error)

	II *Infrastructure
}

func (s *TestSuite) TearDown() {
	s.II.Stop()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	ii, err := s.generator()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	s.II = ii
	// Start the integration ii
	ii.Start()
	// Sleep for a while to allow the networks to be ready
	time.Sleep(20 * time.Second)
}

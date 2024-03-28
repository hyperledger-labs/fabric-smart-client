/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
	. "github.com/onsi/gomega"
)

var NoReplication = &ReplicationOptions{}

type ReplicationOptions struct {
	ReplicationFactors map[string]int
}

func (o *ReplicationOptions) For(name string) []node.Option {
	opts := make([]node.Option, 0, 3)
	if f := o.ReplicationFactors[name]; f > 0 {
		opts = append(opts, fsc.WithReplicationFactor(f))
	}
	return opts
}

func NewTestSuite(generator func() (*Infrastructure, error)) *TestSuite {
	return &TestSuite{
		generator: generator,
		closeFunc: func() {},
	}
}

type TestSuite struct {
	generator func() (*Infrastructure, error)

	closeFunc func()
	II        *Infrastructure
}

func (s *TestSuite) TearDown() {
	s.II.Stop()
	s.closeFunc()
}

func (s *TestSuite) Setup() {
	// Create the integration ii
	ii, err := s.generator()
	Expect(err).NotTo(HaveOccurred())
	s.II = ii
	// Start the integration ii
	ii.Start()
	// Sleep for a while to allow the networks to be ready
	time.Sleep(20 * time.Second)
}

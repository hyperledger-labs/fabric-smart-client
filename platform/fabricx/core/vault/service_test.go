/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger/mock"
	txmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
)

// stubQSProvider returns a fixed query service or error and records the requested network and channel.
type stubQSProvider struct {
	qs               queryservice.QueryService
	err              error
	network, channel string
}

func (p *stubQSProvider) Get(network, channel string) (queryservice.QueryService, error) {
	p.network, p.channel = network, channel
	return p.qs, p.err
}

func TestNew(t *testing.T) {
	t.Parallel()
	cs := &mock.ConfigService{}
	cs.NetworkNameReturns("net1")

	t.Run("provider error", func(t *testing.T) {
		t.Parallel()
		_, err := vault.New(cs, "ch1", &stubQSProvider{err: errors.New("no query service")}, &txmock.MetadataStore{})
		require.ErrorContains(t, err, "failed getting query service")
		require.ErrorContains(t, err, "no query service")
	})

	t.Run("serves state through the provided query service", func(t *testing.T) {
		t.Parallel()
		qs := newMockQueryService()
		qs.setState("ns1", "key1", []byte("value1"), 1)
		p := &stubQSProvider{qs: qs}

		v, err := vault.New(cs, "ch1", p, &txmock.MetadataStore{})
		require.NoError(t, err)
		require.Equal(t, "net1", p.network)
		require.Equal(t, "ch1", p.channel)

		qe, err := v.NewQueryExecutor(context.Background())
		require.NoError(t, err)
		read, err := qe.GetState(context.Background(), "ns1", "key1")
		require.NoError(t, err)
		require.Equal(t, []byte("value1"), read.Raw)
	})
}

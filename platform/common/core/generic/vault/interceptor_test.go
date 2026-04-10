/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/stretchr/testify/require"
)

func TestConcurrency(t *testing.T) {
	t.Parallel()
	qe := mocks.NewMockQE()
	idsr := mocks.MockTxStatusStore{}

	i := newInterceptor(logging.MustGetLogger(), context.Background(), EmptyRWSet(), qe, idsr, "1")
	s, err := i.GetState("ns", "key")
	require.NoError(t, err)
	require.Equal(t, qe.State.Raw, s, "with no opts, getstate should return the FromStorage value (query executor)")

	md, err := i.GetStateMetadata("ns", "key")
	require.NoError(t, err)
	require.Equal(t, qe.Metadata, md, "with no opts, GetStateMetadata should return the FromStorage value (query executor)")

	s, err = i.GetState("ns", "key", driver.FromBoth)
	require.NoError(t, err)
	require.Equal(t, qe.State.Raw, s, "FromBoth should fallback to FromStorage with empty rwset")

	md, err = i.GetStateMetadata("ns", "key", driver.FromBoth)
	require.NoError(t, err)
	require.Equal(t, qe.Metadata, md, "FromBoth should fallback to FromStorage with empty rwset")

	s, err = i.GetState("ns", "key", driver.FromIntermediate)
	require.NoError(t, err)
	require.Equal(t, []byte(nil), s, "FromIntermediate should return empty result from empty rwset")

	md, err = i.GetStateMetadata("ns", "key", driver.FromIntermediate)
	require.NoError(t, err)
	require.Nil(t, md, "FromIntermediate should return empty result from empty rwset")

	// Done in parallel
	wg := sync.WaitGroup{}
	wg.Add(3)
	f := func() {
		i.Done()
		wg.Done()
	}
	go f()
	go f()
	go f()
	wg.Wait()

	_, err = i.GetState("ns", "key")
	require.Error(t, err, "this instance was closed")
}

func TestAddReadAt(t *testing.T) {
	t.Parallel()
	qe := mocks.MockQE{}
	idsr := mocks.MockTxStatusStore{}
	i := newInterceptor(logging.MustGetLogger(), context.Background(), EmptyRWSet(), qe, idsr, "1")

	require.NoError(t, i.AddReadAt("ns", "key", []byte("version")))
	require.Len(t, i.RWs().Reads, 1)
	require.Equal(t, []byte("version"), i.RWs().Reads["ns"]["key"])
}

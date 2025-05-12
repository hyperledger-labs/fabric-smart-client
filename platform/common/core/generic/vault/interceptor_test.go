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
	"github.com/stretchr/testify/assert"
)

func TestConcurrency(t *testing.T) {
	qe := mocks.NewMockQE()
	idsr := mocks.MockTxStatusStore{}

	i := newInterceptor(logging.MustGetLogger(), context.Background(), EmptyRWSet(), qe, idsr, "1")
	s, err := i.GetState("ns", "key")
	assert.NoError(t, err)
	assert.Equal(t, qe.State.Raw, s, "with no opts, getstate should return the FromStorage value (query executor)")

	md, err := i.GetStateMetadata("ns", "key")
	assert.NoError(t, err)
	assert.Equal(t, qe.Metadata, md, "with no opts, GetStateMetadata should return the FromStorage value (query executor)")

	s, err = i.GetState("ns", "key", driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, qe.State.Raw, s, "FromBoth should fallback to FromStorage with empty rwset")

	md, err = i.GetStateMetadata("ns", "key", driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, qe.Metadata, md, "FromBoth should fallback to FromStorage with empty rwset")

	s, err = i.GetState("ns", "key", driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), s, "FromIntermediate should return empty result from empty rwset")

	md, err = i.GetStateMetadata("ns", "key", driver.FromIntermediate)
	assert.NoError(t, err)
	assert.True(t, md == nil, "FromIntermediate should return empty result from empty rwset")

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
	assert.Error(t, err, "this instance was closed")
}

func TestAddReadAt(t *testing.T) {
	qe := mocks.MockQE{}
	idsr := mocks.MockTxStatusStore{}
	i := newInterceptor(logging.MustGetLogger(), context.Background(), EmptyRWSet(), qe, idsr, "1")

	assert.NoError(t, i.AddReadAt("ns", "key", []byte("version")))
	assert.Len(t, i.RWs().Reads, 1)
	assert.Equal(t, []byte("version"), i.RWs().Reads["ns"]["key"])
}

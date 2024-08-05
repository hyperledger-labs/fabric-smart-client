/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

func newMockQE() mockQE {
	return mockQE{
		State: VersionedValue{
			Raw:   []byte("raw"),
			Block: 1,
			TxNum: 1,
		},
		Metadata: map[string][]byte{
			"md": []byte("meta"),
		},
	}
}

type mockQE struct {
	State    VersionedValue
	Metadata map[string][]byte
}

func (qe mockQE) GetStateMetadata(namespace, key string) (map[string][]byte, uint64, uint64, error) {
	return qe.Metadata, 1, 1, nil
}
func (qe mockQE) GetState(namespace, key string) (VersionedValue, error) {
	return qe.State, nil
}
func (qe mockQE) Done() {
}

type mockTXIDStoreReader struct {
}

func (m mockTXIDStoreReader) Iterator(pos interface{}) (driver.TxIDIterator[int], error) {
	panic("not implemented")
}
func (m mockTXIDStoreReader) Get(txID driver.TxID) (int, string, error) {
	return 1, txID, nil
}

func TestConcurrency(t *testing.T) {
	qe := newMockQE()
	idsr := mockTXIDStoreReader{}

	i := newInterceptor(flogging.MustGetLogger("interceptor_test"), qe, idsr, "1")
	s, err := i.GetState("ns", "key")
	assert.NoError(err)
	assert.Equal(qe.State.Raw, s, "with no opts, getstate should return the FromStorage value (query executor)")

	md, err := i.GetStateMetadata("ns", "key")
	assert.NoError(err)
	assert.Equal(qe.Metadata, md, "with no opts, GetStateMetadata should return the FromStorage value (query executor)")

	s, err = i.GetState("ns", "key", driver.FromBoth)
	assert.NoError(err)
	assert.Equal(qe.State.Raw, s, "FromBoth should fallback to FromStorage with empty rwset")

	md, err = i.GetStateMetadata("ns", "key", driver.FromBoth)
	assert.NoError(err)
	assert.Equal(qe.Metadata, md, "FromBoth should fallback to FromStorage with empty rwset")

	s, err = i.GetState("ns", "key", driver.FromIntermediate)
	assert.NoError(err)
	assert.Equal([]byte(nil), s, "FromIntermediate should return empty result from empty rwset")

	md, err = i.GetStateMetadata("ns", "key", driver.FromIntermediate)
	assert.NoError(err)
	assert.True(md == nil, "FromIntermediate should return empty result from empty rwset")

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
	assert.Error(err, "this instance was closed")
}

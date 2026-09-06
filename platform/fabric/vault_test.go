/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

func TestTransientMap(t *testing.T) {
	t.Parallel()

	tm := make(TransientMap)

	require.True(t, tm.IsEmpty())

	require.NoError(t, tm.Set("k1", []byte("v1")))
	require.False(t, tm.IsEmpty())
	require.True(t, tm.Exists("k1"))
	require.Equal(t, []byte("v1"), tm.Get("k1"))

	require.NoError(t, tm.SetState("k2", map[string]string{"a": "b"}))

	var res map[string]string
	require.NoError(t, tm.GetState("k2", &res))
	require.Equal(t, "b", res["a"])

	require.ErrorContains(t, tm.GetState("k3", &res), "does not exists")

	require.NoError(t, tm.Set("k4", []byte("")))
	require.ErrorContains(t, tm.GetState("k4", &res), "is empty")

	// Invalid json: channels cannot be marshalled
	require.ErrorContains(t, tm.SetState("k5", make(chan int)), "unsupported type")
}

func TestRWSet(t *testing.T) {
	t.Parallel()

	mrws := &mock.RWSet{}
	rws := NewRWSet(mrws)

	mrws.NumReadsReturns(2)
	mrws.GetReadAtReturnsOnCall(0, "key1", nil, nil)
	mrws.GetReadAtReturnsOnCall(1, "key2", nil, nil)

	exists, err := rws.KeyExist("key2", "ns1")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, 2, mrws.GetReadAtCallCount())

	mrws.EqualsReturns(nil)
	other := NewRWSet(mrws)
	require.NoError(t, rws.Equals(other, "ns1"))

	require.ErrorContains(t, rws.Equals("not an rwset"), "expected instance of")

	// KeyExist: no read matches the requested key, so it returns false without error
	mrwsMiss := &mock.RWSet{}
	rwsMiss := NewRWSet(mrwsMiss)
	mrwsMiss.NumReadsReturns(1)
	mrwsMiss.GetReadAtReturnsOnCall(0, "otherkey", nil, nil)
	exists, err = rwsMiss.KeyExist("missing", "ns1")
	require.NoError(t, err)
	require.False(t, exists)

	// KeyExist: GetReadAt fails, so the wrapped error is returned
	mrwsErr := &mock.RWSet{}
	rwsErr := NewRWSet(mrwsErr)
	mrwsErr.NumReadsReturns(1)
	mrwsErr.GetReadAtReturnsOnCall(0, "", nil, errors.New("read boom"))
	_, err = rwsErr.KeyExist("k", "ns1")
	require.ErrorContains(t, err, "read boom")
}

type mockEnvSvc struct {
	fdriver.EnvelopeService
	count int
}

func (m *mockEnvSvc) StoreEnvelope(ctx context.Context, id string, env any) error {
	m.count++
	return nil
}

type mockTxSvc struct {
	fdriver.EndorserTransactionService
	count int
}

func (m *mockTxSvc) StoreTransaction(ctx context.Context, id string, raw []byte) error {
	m.count++
	return nil
}

type mockMetaSvc struct {
	fdriver.MetadataService
	count int
}

func (m *mockMetaSvc) StoreTransient(ctx context.Context, id string, tm fdriver.TransientMap) error {
	m.count++
	return nil
}

func TestVault(t *testing.T) {
	t.Parallel()

	mockCh := &mock.Channel{}
	mockV := &mock.Vault{}
	mockCh.VaultReturns(mockV)

	mes := &mockEnvSvc{}
	mts := &mockTxSvc{}
	mms := &mockMetaSvc{}

	mockCh.EnvelopeServiceReturns(mes)
	mockCh.TransactionServiceReturns(mts)
	mockCh.MetadataServiceReturns(mms)

	vault := newVault(mockCh)

	ctx := t.Context()

	mockV.NewQueryExecutorReturns(nil, nil)
	_, err := vault.NewQueryExecutor(ctx)
	require.NoError(t, err)

	mockV.StatusReturns(fdriver.Valid, "dep", nil)
	code, dep, err := vault.Status(ctx, "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Valid, code)
	require.Equal(t, "dep", dep)

	mrws := &mock.RWSet{}
	mockV.NewRWSetReturns(mrws, nil)
	rws, err := vault.NewRWSet(ctx, "tx1")
	require.NoError(t, err)
	require.NotNil(t, rws)

	mockV.NewRWSetFromBytesReturns(mrws, nil)
	rws, err = vault.NewRWSetFromBytes(ctx, "tx1", []byte("rwset"))
	require.NoError(t, err)
	require.NotNil(t, rws)

	mockV.InspectRWSetReturns(mrws, nil)
	rws, err = vault.InspectRWSet(ctx, []byte("rwset"), "ns1")
	require.NoError(t, err)
	require.NotNil(t, rws)

	require.NoError(t, vault.StoreEnvelope(ctx, "tx1", []byte("env")))
	require.Equal(t, 1, mes.count)

	require.NoError(t, vault.StoreTransaction(ctx, "tx1", []byte("tx")))
	require.Equal(t, 1, mts.count)

	require.NoError(t, vault.StoreTransient(ctx, "tx1", make(TransientMap)))
	require.Equal(t, 1, mms.count)

	// error paths: the vault wrapper propagates the underlying store errors unchanged
	mockV.NewRWSetReturns(nil, errors.New("newrwset boom"))
	_, err = vault.NewRWSet(ctx, "tx1")
	require.ErrorContains(t, err, "newrwset boom")

	mockV.NewRWSetFromBytesReturns(nil, errors.New("frombytes boom"))
	_, err = vault.NewRWSetFromBytes(ctx, "tx1", []byte("rwset"))
	require.ErrorContains(t, err, "frombytes boom")

	mockV.InspectRWSetReturns(nil, errors.New("inspect boom"))
	_, err = vault.InspectRWSet(ctx, []byte("rwset"), "ns1")
	require.ErrorContains(t, err, "inspect boom")
}

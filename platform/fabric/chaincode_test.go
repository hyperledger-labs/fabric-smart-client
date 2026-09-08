/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestEnvelope(t *testing.T) {
	t.Parallel()

	menv := &mock.Envelope{}
	env := NewEnvelope(menv)

	menv.StringReturns("myenv")
	require.Equal(t, "myenv", env.String())

	menv.BytesReturns([]byte("bytes"), nil)
	raw, err := env.MarshalJSON()
	require.NoError(t, err)

	var encoded []byte
	require.NoError(t, json.Unmarshal(raw, &encoded))
	require.Equal(t, []byte("bytes"), encoded)

	menv.FromBytesReturns(nil)
	require.NoError(t, env.UnmarshalJSON(raw))
}

type mockChaincode struct {
	driver.Chaincode
	mci *dummyChaincodeInvocation
	mcd *dummyChaincodeDiscover
}

func (m *mockChaincode) NewInvocation(_ string, _ ...any) driver.ChaincodeInvocation {
	return m.mci
}

func (m *mockChaincode) NewDiscover() driver.ChaincodeDiscover {
	return m.mcd
}

func (*mockChaincode) IsAvailable() (bool, error) {
	return true, nil
}

func (*mockChaincode) Version() (string, error) {
	return "v1", nil
}

func TestChaincode(t *testing.T) {
	t.Parallel()

	mfns := &mock.FabricNetworkService{}
	mlm := &mock.LocalMembership{}
	mfns.LocalMembershipReturns(mlm)
	mlm.DefaultIdentityReturns([]byte("defaultid"))

	mci := &dummyChaincodeInvocation{}
	mcd := &dummyChaincodeDiscover{}
	mcc := &mockChaincode{mci: mci, mcd: mcd}

	cc := &Chaincode{chaincode: mcc, fns: mfns}

	inv := cc.Invoke("f1", "a1")
	require.NotNil(t, inv)
	require.Equal(t, 1, mci.withSignerIdentityCallCount)

	query := cc.Query("f2", "a2")
	require.NotNil(t, query)
	require.Equal(t, 2, mci.withSignerIdentityCallCount)

	endorse := cc.Endorse("f3", "a3")
	require.NotNil(t, endorse)
	require.Equal(t, 3, mci.withSignerIdentityCallCount)

	disc := cc.Discover()
	require.NotNil(t, disc)

	av, err := cc.IsAvailable()
	require.NoError(t, err)
	require.True(t, av)

	v, err := cc.Version()
	require.NoError(t, err)
	require.Equal(t, "v1", v)
}

func TestChaincodeInvocation(t *testing.T) {
	t.Parallel()

	mci := &dummyChaincodeInvocation{
		submitResultID: "tx1",
		submitResult:   []byte("res"),
	}
	inv := &ChaincodeInvocation{ChaincodeInvocation: mci}

	id, res, err := inv.Call()
	require.NoError(t, err)
	require.Equal(t, "tx1", id)
	require.Equal(t, []byte("res"), res)

	ctx := t.Context()
	inv.WithContext(ctx)

	_, err = inv.WithTransientEntry("k", "v")
	require.NoError(t, err)

	type testCase struct {
		name  string
		call  func()
		check func() int
	}

	tests := []testCase{
		{"WithSignerIdentity", func() { inv.WithInvokerIdentity([]byte("id")) }, func() int { return mci.withSignerIdentityCallCount }},
		{"WithEndorsersByMSPIDs", func() { inv.WithEndorsersByMSPIDs("msp1") }, func() int { return mci.withEndorsersByMSPIDsCallCount }},
		{"WithEndorsersFromMyOrg", func() { inv.WithEndorsersFromMyOrg() }, func() int { return mci.withEndorsersFromMyOrgCallCount }},
		{"WithNumRetries", func() { inv.WithNumRetries(5) }, func() int { return mci.withNumRetriesCallCount }},
		{"WithRetrySleep", func() { inv.WithRetrySleep(time.Second) }, func() int { return mci.withRetrySleepCallCount }},
		{"WithDiscoveredEndorsersByEndpoints", func() { inv.WithDiscoveredEndorsersByEndpoints("ep1") }, func() int { return mci.withDiscoveredEndorsersByEndpointsCallCount }},
		{"WithTxID", func() { inv.WithTxID(driver.TxIDComponents{Nonce: []byte("n"), Creator: []byte("c")}) }, func() int { return mci.withTxIDCallCount }},
		{"WithMatchEndorsementPolicy", func() { inv.WithMatchEndorsementPolicy() }, func() int { return mci.withMatchEndorsementPolicyCallCount }},
		{"WithImplicitCollections", func() { inv.WithImplicitCollections("msp1") }, func() int { return mci.withImplicitCollectionsCallCount }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := tc.check()
			tc.call()
			require.Equal(t, before+1, tc.check())
		})
	}
}

type dummyChaincodeInvocation struct {
	driver.ChaincodeInvocation
	withSignerIdentityCallCount                 int
	withEndorsersByMSPIDsCallCount              int
	withEndorsersFromMyOrgCallCount             int
	withNumRetriesCallCount                     int
	withRetrySleepCallCount                     int
	withDiscoveredEndorsersByEndpointsCallCount int
	withTxIDCallCount                           int
	withMatchEndorsementPolicyCallCount         int
	withImplicitCollectionsCallCount            int
	submitResultID                              string
	submitResult                                []byte
	queryResult                                 []byte
	endorseResult                               driver.Envelope
}

func (d *dummyChaincodeInvocation) Submit() (string, []byte, error) {
	return d.submitResultID, d.submitResult, nil
}

func (d *dummyChaincodeInvocation) Query() ([]byte, error) {
	return d.queryResult, nil
}

func (d *dummyChaincodeInvocation) Endorse() (driver.Envelope, error) {
	return d.endorseResult, nil
}

func (d *dummyChaincodeInvocation) WithSignerIdentity(_ view.Identity) driver.ChaincodeInvocation {
	d.withSignerIdentityCallCount++
	return d
}

func (d *dummyChaincodeInvocation) WithContext(context.Context) driver.ChaincodeInvocation { return d }

func (d *dummyChaincodeInvocation) WithTransientEntry(string, any) (driver.ChaincodeInvocation, error) {
	return d, nil
}

func (d *dummyChaincodeInvocation) WithEndorsersByMSPIDs(...string) driver.ChaincodeInvocation {
	d.withEndorsersByMSPIDsCallCount++
	return d
}

func (d *dummyChaincodeInvocation) WithEndorsersFromMyOrg() driver.ChaincodeInvocation {
	d.withEndorsersFromMyOrgCallCount++
	return d
}

func (d *dummyChaincodeInvocation) WithNumRetries(uint) driver.ChaincodeInvocation {
	d.withNumRetriesCallCount++
	return d
}

func (d *dummyChaincodeInvocation) WithRetrySleep(time.Duration) driver.ChaincodeInvocation {
	d.withRetrySleepCallCount++
	return d
}

func (d *dummyChaincodeInvocation) WithDiscoveredEndorsersByEndpoints(...string) driver.ChaincodeInvocation {
	d.withDiscoveredEndorsersByEndpointsCallCount++
	return d
}

func (d *dummyChaincodeInvocation) WithTxID(driver.TxIDComponents) driver.ChaincodeInvocation {
	d.withTxIDCallCount++
	return d
}

func (d *dummyChaincodeInvocation) WithMatchEndorsementPolicy() driver.ChaincodeInvocation {
	d.withMatchEndorsementPolicyCallCount++
	return d
}

func (d *dummyChaincodeInvocation) WithImplicitCollections(...string) driver.ChaincodeInvocation {
	d.withImplicitCollectionsCallCount++
	return d
}

func TestChaincodeQuery(t *testing.T) {
	t.Parallel()

	mci := &dummyChaincodeInvocation{
		queryResult: []byte("res"),
	}
	q := &ChaincodeQuery{ChaincodeInvocation: mci}

	res, err := q.Call()
	require.NoError(t, err)
	require.Equal(t, []byte("res"), res)

	ctx := t.Context()
	q.WithContext(ctx)

	_, err = q.WithTransientEntry("k", "v")
	require.NoError(t, err)

	q.WithDiscoveredEndorsersByEndpoints("ep1")
	q.WithEndorsersByMSPIDs("msp1")
	q.WithEndorsersFromMyOrg()
	q.WithInvokerIdentity([]byte("id"))
	require.Equal(t, 1, mci.withSignerIdentityCallCount)

	q.WithTxID(TxID{Nonce: []byte("n"), Creator: []byte("c")})
	q.WithMatchEndorsementPolicy()
	q.WithNumRetries(5)
	q.WithRetrySleep(time.Second)
}

func TestChaincodeEndorse(t *testing.T) {
	t.Parallel()

	menv := &mock.Envelope{}
	mci := &dummyChaincodeInvocation{
		endorseResult: menv,
	}
	e := &ChaincodeEndorse{ChaincodeInvocation: mci}

	env, err := e.Call()
	require.NoError(t, err)
	require.NotNil(t, env)

	ctx := t.Context()
	e.WithContext(ctx)

	_, err = e.WithTransientEntry("k", "v")
	require.NoError(t, err)

	_, err = e.WithTransientEntries(map[string]any{"k2": "v2"})
	require.NoError(t, err)

	e.WithEndorsersByMSPIDs("msp1")
	e.WithEndorsersFromMyOrg()
	e.WithInvokerIdentity([]byte("id"))
	require.Equal(t, 1, mci.withSignerIdentityCallCount)
	e.WithTxID(TxID{Nonce: []byte("n"), Creator: []byte("c")})
	e.WithImplicitCollections("msp1")
	e.WithNumRetries(5)
	e.WithRetrySleep(time.Second)
}

type dummyChaincodeDiscover struct {
	driver.ChaincodeDiscover
	callResult []driver.DiscoveredPeer
}

func (d *dummyChaincodeDiscover) Call() ([]driver.DiscoveredPeer, error)                { return d.callResult, nil }
func (d *dummyChaincodeDiscover) WithFilterByMSPIDs(...string) driver.ChaincodeDiscover { return d }

func TestChaincodeDiscover(t *testing.T) {
	t.Parallel()

	mcd := &dummyChaincodeDiscover{
		callResult: []driver.DiscoveredPeer{{Identity: []byte("p1")}},
	}
	d := &ChaincodeDiscover{ChaincodeDiscover: mcd}

	peers, err := d.Call()
	require.NoError(t, err)
	require.Len(t, peers, 1)

	d.WithFilterByMSPIDs("msp1")

	ids := DiscoveredIdentities(peers)
	require.Len(t, ids, 1)
	require.Equal(t, []byte("p1"), []byte(ids[0]))
}

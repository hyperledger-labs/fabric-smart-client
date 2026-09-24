/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	qsmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
)

type (
	stubConfigProvider        struct{ config.Provider }
	stubEnvelopeStore         struct{ fdriver.EnvelopeStore }
	stubMetadataStore         struct{ fdriver.MetadataStore }
	stubEndorseTxStore        struct{ fdriver.EndorseTxStore }
	stubChannelConfigProvider struct{ fdriver.ChannelConfigProvider }
)

func TestNewProvider(t *testing.T) {
	t.Parallel()

	for _, useFiltered := range []bool{false, true} {
		cp := &stubConfigProvider{}
		eKVS := &stubEnvelopeStore{}
		mKVS := &stubMetadataStore{}
		etKVS := &stubEndorseTxStore{}
		ccProv := &stubChannelConfigProvider{}
		qsProv := &mockQueryServiceProvider{}
		lmProv := &mockListenerManagerProvider{}

		vaultCtor := func(string, fdriver.ConfigService, cdriver.VaultStore) (fdriver.Vault, error) { return nil, nil }
		ledgerCtor := LedgerConstructor(func(string, fdriver.FabricNetworkService, fdriver.ChaincodeManager) (fdriver.Ledger, error) {
			return nil, nil
		})
		rwsetCtor := RWSetLoaderConstructor(func(string, fdriver.FabricNetworkService, fdriver.EnvelopeService, fdriver.EndorserTransactionService, fdriver.RWSetInspector) (fdriver.RWSetLoader, error) {
			return nil, nil
		})
		deliveryCtor := DeliveryConstructor(func(fdriver.FabricNetworkService, string, delivery.Services, fdriver.Ledger, delivery.Vault, fdriver.BlockCallback) (generic.DeliveryService, error) {
			return nil, nil
		})
		membershipCtor := MembershipConstructor(func(string) fdriver.MembershipService { return nil })

		p := NewProvider(
			cp, eKVS, mKVS, etKVS, multiplexed.Driver{},
			vaultCtor, ccProv, ledgerCtor, rwsetCtor,
			deliveryCtor, membershipCtor, useFiltered, qsProv, lmProv,
		)
		require.NotNil(t, p)

		assert.Same(t, cp, p.configProvider)
		assert.Same(t, eKVS, p.envelopeKVS)
		assert.Same(t, mKVS, p.metadataKVS)
		assert.Same(t, etKVS, p.endorserTxKVS)
		assert.NotNil(t, p.newVault)
		assert.Same(t, ccProv, p.channelConfigProvider)
		assert.NotNil(t, p.newLedger)
		assert.NotNil(t, p.newRWSetLoader)
		assert.NotNil(t, p.newDelivery)
		assert.NotNil(t, p.newMembership)
		assert.Equal(t, useFiltered, p.useFilteredDelivery)
		assert.Same(t, qsProv, p.queryServiceProvider)
		assert.Same(t, lmProv, p.listenerManagerProvider)
	}

	t.Run("nil fields stored cleanly", func(t *testing.T) {
		t.Parallel()
		pNil := NewProvider(nil, nil, nil, nil, multiplexed.Driver{}, nil, nil, nil, nil, nil, nil, false, nil, nil)
		require.NotNil(t, pNil)
		assert.Nil(t, pNil.configProvider)
		assert.Nil(t, pNil.envelopeKVS)
		assert.Nil(t, pNil.metadataKVS)
		assert.Nil(t, pNil.endorserTxKVS)
		assert.Nil(t, pNil.newVault)
		assert.Nil(t, pNil.channelConfigProvider)
		assert.Nil(t, pNil.newLedger)
		assert.Nil(t, pNil.newRWSetLoader)
		assert.Nil(t, pNil.newDelivery)
		assert.Nil(t, pNil.newMembership)
		assert.False(t, pNil.useFilteredDelivery)
		assert.Nil(t, pNil.queryServiceProvider)
		assert.Nil(t, pNil.listenerManagerProvider)
	})
}

// --- Stubs for NewChannel testing ---

type stubVault struct {
	mu sync.Mutex
	fdriver.Vault
	closed   bool
	closeErr error
}

func (*stubVault) Statuses(_ context.Context, _ ...cdriver.TxID) ([]cdriver.TxValidationStatus[fdriver.ValidationCode], error) {
	return nil, nil
}

func (v *stubVault) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.closed = true
	return v.closeErr
}

func (v *stubVault) IsClosed() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.closed
}

type stubDelivery struct {
	mu sync.Mutex
	generic.DeliveryService
	stopped bool
}

func (d *stubDelivery) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
}

func (d *stubDelivery) IsStopped() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopped
}

type stubRWSetLoader struct {
	fdriver.RWSetLoader
}

type stubLedger struct {
	fdriver.Ledger
}

type mockListenerManagerProvider struct {
	mu      sync.Mutex
	manager finality.ListenerManager
	err     error
}

func (m *mockListenerManagerProvider) NewManager(string, string) (finality.ListenerManager, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.manager, m.err
}

type stubLocalMembership struct {
	fdriver.LocalMembership
}

func (*stubLocalMembership) DefaultSigningIdentity() fdriver.SigningIdentity {
	return nil
}

func newTestNetwork() *stubFNS {
	mockCS := &mock.ConfigService{}
	mockCS.IsSetStub = func(k string) bool {
		return k == "configMonitor.maxRetries"
	}
	mockCS.GetIntReturns(0)

	return &stubFNS{
		name:            "testnet",
		configService:   mockCS,
		orderingService: &dummyOrdering{},
		localMembership: &stubLocalMembership{},
	}
}

func defaultTestProvider(
	v *stubVault,
	d *stubDelivery,
	qs queryservice.QueryService,
	lm finality.ListenerManager,
) *provider {
	return NewProvider(
		nil, nil, nil, nil, multiplexed.Driver{},
		func(string, fdriver.ConfigService, cdriver.VaultStore) (fdriver.Vault, error) {
			return v, nil
		},
		nil,
		func(string, fdriver.FabricNetworkService, fdriver.ChaincodeManager) (fdriver.Ledger, error) {
			return &stubLedger{}, nil
		},
		func(string, fdriver.FabricNetworkService, fdriver.EnvelopeService, fdriver.EndorserTransactionService, fdriver.RWSetInspector) (fdriver.RWSetLoader, error) {
			return &stubRWSetLoader{}, nil
		},
		func(fdriver.FabricNetworkService, string, delivery.Services, fdriver.Ledger, delivery.Vault, fdriver.BlockCallback) (generic.DeliveryService, error) {
			return d, nil
		},
		func(string) fdriver.MembershipService {
			return &stubMembershipService{}
		},
		false,
		&mockQueryServiceProvider{qs: qs},
		&mockListenerManagerProvider{manager: lm},
	)
}

// --- NewChannel Tests ---

func TestNewChannel_FailingVault(t *testing.T) {
	t.Parallel()

	p := defaultTestProvider(&stubVault{}, &stubDelivery{}, &qsmock.QueryService{}, &mockListenerManager{})
	p.newVault = func(string, fdriver.ConfigService, cdriver.VaultStore) (fdriver.Vault, error) {
		return nil, assert.AnError
	}

	nw := newTestNetwork()
	ch, err := p.NewChannel(nw, "mychannel", false)
	require.ErrorContains(t, err, "failed creating vault for channel [mychannel]")
	assert.Nil(t, ch)
}

func TestNewChannel_FailingRWSetLoader(t *testing.T) {
	t.Parallel()

	p := defaultTestProvider(&stubVault{}, &stubDelivery{}, &qsmock.QueryService{}, &mockListenerManager{})
	p.newRWSetLoader = func(string, fdriver.FabricNetworkService, fdriver.EnvelopeService, fdriver.EndorserTransactionService, fdriver.RWSetInspector) (fdriver.RWSetLoader, error) {
		return nil, assert.AnError
	}

	nw := newTestNetwork()
	ch, err := p.NewChannel(nw, "mychannel", false)
	require.ErrorContains(t, err, "failed creating RWSetLoader for channel [mychannel]")
	assert.Nil(t, ch)
}

func TestNewChannel_FailingLedger(t *testing.T) {
	t.Parallel()

	p := defaultTestProvider(&stubVault{}, &stubDelivery{}, &qsmock.QueryService{}, &mockListenerManager{})
	p.newLedger = func(string, fdriver.FabricNetworkService, fdriver.ChaincodeManager) (fdriver.Ledger, error) {
		return nil, assert.AnError
	}

	nw := newTestNetwork()
	ch, err := p.NewChannel(nw, "mychannel", false)
	require.ErrorContains(t, err, "failed creating ledger for channel [mychannel]")
	assert.Nil(t, ch)
}

func TestNewChannel_FailingDelivery(t *testing.T) {
	t.Parallel()

	p := defaultTestProvider(&stubVault{}, &stubDelivery{}, &qsmock.QueryService{}, &mockListenerManager{})
	p.newDelivery = func(fdriver.FabricNetworkService, string, delivery.Services, fdriver.Ledger, delivery.Vault, fdriver.BlockCallback) (generic.DeliveryService, error) {
		return nil, assert.AnError
	}

	nw := newTestNetwork()
	ch, err := p.NewChannel(nw, "mychannel", false)
	require.ErrorContains(t, err, "failed creating delivery for channel [mychannel]")
	assert.Nil(t, ch)
}

func TestNewChannel_FailingListenerManager(t *testing.T) {
	t.Parallel()

	p := defaultTestProvider(&stubVault{}, &stubDelivery{}, &qsmock.QueryService{}, &mockListenerManager{})
	p.listenerManagerProvider = &mockListenerManagerProvider{err: assert.AnError}

	nw := newTestNetwork()
	ch, err := p.NewChannel(nw, "mychannel", false)
	require.ErrorContains(t, err, "failed creating listener manager for channel [mychannel]")
	assert.Nil(t, ch)
}

func TestNewChannel_FailingConfigMonitor(t *testing.T) {
	t.Parallel()

	p := defaultTestProvider(&stubVault{}, &stubDelivery{}, &qsmock.QueryService{}, &mockListenerManager{})
	p.queryServiceProvider = &mockQueryServiceProvider{err: assert.AnError}

	nw := newTestNetwork()
	ch, err := p.NewChannel(nw, "mychannel", false)
	require.ErrorContains(t, err, "failed starting channel config monitor for channel [mychannel]")
	assert.Nil(t, ch)
}

func TestNewChannel_SuccessAndClose(t *testing.T) {
	t.Parallel()

	v := &stubVault{}
	d := &stubDelivery{}
	qs := &qsmock.QueryService{}
	qs.GetConfigTransactionReturns(nil, assert.AnError)
	lm := &mockListenerManager{}

	p := defaultTestProvider(v, d, qs, lm)
	nw := newTestNetwork()

	ch, err := p.NewChannel(nw, "mychannel", false)
	require.NoError(t, err)
	require.NotNil(t, ch)
	assert.Equal(t, "mychannel", ch.Name())
	assert.Equal(t, v, ch.Vault())
	assert.NotNil(t, ch.Finality())

	// Close the channel and assert resources are released
	require.NoError(t, ch.Close())
	assert.True(t, v.IsClosed())
	assert.True(t, d.IsStopped())

	chImpl, ok := ch.(*channel)
	require.True(t, ok)
	assert.False(t, chImpl.Monitor.IsRunning())
}

func TestChannel_Close_ErrorPropagation(t *testing.T) {
	t.Parallel()

	t.Run("vault close error", func(t *testing.T) {
		t.Parallel()

		v := &stubVault{closeErr: errors.New("vault close failure")}
		d := &stubDelivery{}
		qs := &qsmock.QueryService{}
		qs.GetConfigTransactionReturns(nil, assert.AnError)
		lm := &mockListenerManager{}

		p := defaultTestProvider(v, d, qs, lm)
		nw := newTestNetwork()

		ch, err := p.NewChannel(nw, "mychannel", false)
		require.NoError(t, err)

		err = ch.Close()
		require.ErrorContains(t, err, "vault close failure")
		assert.True(t, v.IsClosed())
		assert.True(t, d.IsStopped())
	})

	t.Run("monitor stop error", func(t *testing.T) {
		t.Parallel()

		v := &stubVault{}
		d := &stubDelivery{}
		qs := &qsmock.QueryService{}
		qs.GetConfigTransactionReturns(nil, assert.AnError)
		lm := &mockListenerManager{}

		p := defaultTestProvider(v, d, qs, lm)
		nw := newTestNetwork()

		ch, err := p.NewChannel(nw, "mychannel", false)
		require.NoError(t, err)

		chImpl, ok := ch.(*channel)
		require.True(t, ok)
		// Stop monitor first so that subsequent Close encounters "monitor is not running" error
		require.NoError(t, chImpl.Monitor.Stop())

		err = ch.Close()
		require.ErrorContains(t, err, "monitor is not running")
		assert.True(t, v.IsClosed())
		assert.True(t, d.IsStopped())
	})
}

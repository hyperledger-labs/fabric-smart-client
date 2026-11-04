/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestBuilderWithNilServiceProvider(t *testing.T) {
	t.Parallel()

	for name, build := range map[string]func() (*Builder, *Transaction, error){
		"NewBuilder": func() (*Builder, *Transaction, error) {
			b, err := NewBuilder(nil)
			return b, nil, err
		},
		"NewTransactionWith": func() (*Builder, *Transaction, error) {
			return NewTransactionWith(context.Background(), nil, "", "", nil)
		},
		"NewTransactionFromEnvelopeBytes": func() (*Builder, *Transaction, error) {
			return NewTransactionFromEnvelopeBytes(context.Background(), nil, []byte("raw"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			builder, tx, err := build()
			require.EqualError(t, err, "service provider must be set")
			require.Nil(t, builder)
			require.Nil(t, tx)
		})
	}

	t.Run("zero value Builder", func(t *testing.T) {
		t.Parallel()
		var b Builder
		for name, build := range map[string]func() (*Transaction, error){
			"NewTransaction":          func() (*Transaction, error) { return b.NewTransaction(context.Background()) },
			"NewTransactionFromBytes": func() (*Transaction, error) { return b.NewTransactionFromBytes([]byte("raw")) },
			"NewTransactionFromEnvelopeBytes": func() (*Transaction, error) {
				return b.NewTransactionFromEnvelopeBytes(context.Background(), []byte("raw"))
			},
			"NewTransactionWithIdentity": func() (*Transaction, error) { return b.NewTransactionWithIdentity(nil) },
		} {
			tx, err := build()
			require.EqualError(t, err, "service provider must be set", name)
			require.Nil(t, tx, name)
		}
	})
}

// countingMetrics counts every Add on its counters by label values.
type countingMetrics struct {
	metrics.Provider
	mu     sync.Mutex
	counts map[string]float64
}

func (p *countingMetrics) NewCounter(metrics.CounterOpts) metrics.Counter {
	p.counts = map[string]float64{}
	return &countingCounter{p: p}
}

type countingCounter struct {
	p      *countingMetrics
	labels string
}

func (c *countingCounter) With(labelValues ...string) metrics.Counter {
	return &countingCounter{p: c.p, labels: c.labels + strings.Join(labelValues, "|")}
}

func (c *countingCounter) Add(delta float64) {
	c.p.mu.Lock()
	defer c.p.mu.Unlock()
	c.p.counts[c.labels] += delta
}

func (p *countingMetrics) count(kind security.Kind) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.counts["kind|"+string(kind)]
}

// receiveFixture is a view context on two networks, net1 (the default) and net2,
// that both have a channel ch1, with different genesis hashes. Each network
// decodes every received transaction into its own transaction, dtx on net1 and
// dtx2 on net2. The session of the context is from caller.
type receiveFixture struct {
	ctx     *mock.Context
	dtx     *mock.Transaction
	tm      *mock.TransactionManager
	cm      *mock.ChannelMembership
	conf    *mock.ConfigService
	dtx2    *mock.Transaction
	cm2     *mock.ChannelMembership
	ch2     *mock.Channel
	nets    [2]*receiveNetwork
	fnsp    *mock.FabricNetworkServiceProvider
	metrics *countingMetrics
}

var (
	net1Key = &driver.ChannelKey{Platform: "generic", Channel: "ch1", GenesisHash: []byte("genesis1")}
	net2Key = &driver.ChannelKey{Platform: "generic", Channel: "ch1", GenesisHash: []byte("genesis2")}
)

// keyedRaw returns the bytes of a serialized transaction that carries key.
func keyedRaw(t *testing.T, key *driver.ChannelKey) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"Key": key})
	require.NoError(t, err)
	return raw
}

// legacyRaw is the bytes of a serialized transaction without a key.
var legacyRaw = []byte("{}")

func newReceiveFixture(t *testing.T, caller view.Identity) *receiveFixture {
	t.Helper()
	f := &receiveFixture{ctx: &mock.Context{}, metrics: &countingMetrics{}}
	fns1, _ := newReceiveNetwork("net1", net1Key)
	fns2, ch2 := newReceiveNetwork("net2", net2Key)
	f.dtx, f.tm, f.cm, f.conf = fns1.dtx, fns1.tm, fns1.cm, fns1.conf
	f.dtx2, f.cm2, f.ch2 = fns2.dtx, fns2.cm, ch2
	f.nets = [2]*receiveNetwork{fns1, fns2}

	fnsp := &mock.FabricNetworkServiceProvider{}
	f.fnsp = fnsp
	fnsp.NamesReturns([]string{"net1", "net2"})
	fnsp.DefaultNameReturns("net1")
	fnsp.FabricNetworkServiceCalls(func(id string) (driver.FabricNetworkService, error) {
		switch id {
		case "", "net1":
			return fns1.FabricNetworkService, nil
		case "net2":
			return fns2.FabricNetworkService, nil
		}
		return nil, errors.Errorf("network [%s] not found", id)
	})
	nsp := fabric.NewNetworkServiceProvider(fnsp, nil)
	rejections := security.NewRejections(f.metrics)

	f.ctx.ContextReturns(t.Context())
	f.ctx.GetServiceCalls(func(v any) (any, error) {
		switch v {
		case reflect.TypeFor[*fabric.NetworkServiceProvider]():
			return nsp, nil
		case reflect.TypeFor[*security.Rejections]():
			return rejections, nil
		}
		return nil, errors.New("service not found")
	})
	session := &mock.Session{}
	session.InfoReturns(view.SessionInfo{Caller: caller})
	f.ctx.SessionReturns(session)
	return f
}

// receiveNetwork is a network of receiveFixture with the mocks a test inspects.
type receiveNetwork struct {
	*mock.FabricNetworkService
	dtx  *mock.Transaction
	tm   *mock.TransactionManager
	cm   *mock.ChannelMembership
	conf *mock.ConfigService
}

// newReceiveNetwork returns a network named name with one channel ch1 whose key
// is key, and that channel.
func newReceiveNetwork(name string, key *driver.ChannelKey) (*receiveNetwork, *mock.Channel) {
	n := &receiveNetwork{
		FabricNetworkService: &mock.FabricNetworkService{},
		dtx:                  &mock.Transaction{},
		tm:                   &mock.TransactionManager{},
		cm:                   &mock.ChannelMembership{},
		conf:                 &mock.ConfigService{},
	}
	n.dtx.NetworkReturns(name)
	n.dtx.ChannelReturns("ch1")
	n.dtx.IDReturns(testTxID)
	n.dtx.NonceReturns(testNonce)
	n.dtx.CreatorReturns(testCreator)
	n.tm.NewTransactionFromBytesReturns(n.dtx, nil)
	n.tm.NewTransactionFromEnvelopeBytesReturns(n.dtx, nil)
	n.tm.ComputeTxIDStub = func(id *driver.TxIDComponents) string { return protoutil.ComputeTxID(id.Nonce, id.Creator) }
	n.conf.DriverNameReturns("generic")
	n.conf.ChannelIDsReturns([]string{"ch1"})

	ch := &mock.Channel{}
	ch.NameReturns("ch1")
	ch.ChannelMembershipReturns(n.cm)
	ch.ChannelKeyReturns(key, nil)
	n.NameReturns(name)
	n.ChannelReturns(ch, nil)
	n.TransactionManagerReturns(n.tm)
	n.ConfigServiceReturns(n.conf)
	n.LocalMembershipReturns(&mock.LocalMembership{})
	return n, ch
}

func TestNewTransaction_HonoursWithTxID(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, testCreator)
	var txIDs []string
	f.tm.NewTransactionStub = func(_ context.Context, _ driver.TransactionType, _ view.Identity, _ []byte, txID string, _ string, _ []byte) (driver.Transaction, error) {
		txIDs = append(txIDs, txID)
		tx := &mock.Transaction{}
		tx.IDReturns(txID)
		return tx, nil
	}
	builder, err := NewBuilder(f.ctx)
	require.NoError(t, err)

	tx, err := builder.NewTransaction(t.Context(), fabric.WithCreator(testCreator), fabric.WithNonce(testNonce), fabric.WithTxID("tx1"))
	require.NoError(t, err)
	defer tx.Close()
	require.Equal(t, "tx1", tx.ID())
	require.Equal(t, []string{"tx1"}, txIDs)
}

func TestNewTransaction_DerivesTxIDWithoutWithTxID(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, testCreator)
	var txIDs []string
	f.tm.NewTransactionStub = func(_ context.Context, _ driver.TransactionType, _ view.Identity, _ []byte, txID string, _ string, _ []byte) (driver.Transaction, error) {
		txIDs = append(txIDs, txID)
		return &mock.Transaction{}, nil
	}
	builder, err := NewBuilder(f.ctx)
	require.NoError(t, err)

	tx, err := builder.NewTransaction(t.Context(), fabric.WithCreator(testCreator), fabric.WithNonce(testNonce))
	require.NoError(t, err)
	defer tx.Close()
	// No txID is passed down, so the transaction manager derives it from nonce and creator.
	require.Equal(t, []string{""}, txIDs)
}

func TestNewTransactionFromBytes_RejectsForgedTxID(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.dtx.IDReturns("forged")

	_, _, err := NewTransactionFromBytes(f.ctx, legacyRaw)
	require.ErrorIs(t, err, ErrInvalidTransaction)
	require.InDelta(t, 1, f.metrics.count(security.KindInvalidTx), 0)
	// The error, which is also the reason the rejection reports, names the cause.
	require.Contains(t, err.Error(), `tx "forged": txID is not derived from nonce and creator`)
}

func TestNewTransactionFromBytes_Valid(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))

	_, tx, err := NewTransactionFromBytes(f.ctx, legacyRaw)
	require.NoError(t, err)
	require.Equal(t, testTxID, tx.ID())
	require.Equal(t, testCreator, []byte(f.cm.IsValidArgsForCall(0)))
}

// A channel without configuration is a retryable condition, not a rejection.
func TestNewTransactionFromBytes_MSPNotInitializedNotCounted(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.cm.IsValidReturns(driver.ErrNotInitialized)

	_, _, err := NewTransactionFromBytes(f.ctx, legacyRaw)
	require.ErrorIs(t, err, driver.ErrNotInitialized)
	require.Zero(t, f.metrics.count(security.KindInvalidTx))
}

// Fabric-x does not derive the txID from nonce and creator.
func TestNewTransactionFromBytes_FabricxSkipsTxIDCheck(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.conf.DriverNameReturns(fabricxDriverName)
	f.dtx.IDReturns("fabricx-txid")

	_, _, err := NewTransactionFromBytes(f.ctx, legacyRaw)
	require.NoError(t, err)
}

// A transaction read from a session the view opened itself is reported under
// the peer passed in, so NewTransactionFromPeer never consults the session of
// the view context.
func TestNewTransactionFromPeer_DoesNotUseContextSession(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("carol"))
	f.dtx.IDReturns("forged")

	_, _, err := NewTransactionFromPeer(f.ctx, []byte("dave"), legacyRaw)
	require.ErrorIs(t, err, ErrInvalidTransaction)
	require.InDelta(t, 1, f.metrics.count(security.KindInvalidTx), 0)
	require.Zero(t, f.ctx.SessionCallCount())
}

// A transaction is decoded and validated on the network its key names, whichever
// network is the default.
func TestNewTransactionFromBytes_RoutesByChannelKey(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		key        *driver.ChannelKey
		network    string
		validateOn func(*receiveFixture) *mock.ChannelMembership
		otherOn    func(*receiveFixture) *mock.ChannelMembership
	}{
		"default network": {net1Key, "net1", func(f *receiveFixture) *mock.ChannelMembership { return f.cm }, func(f *receiveFixture) *mock.ChannelMembership { return f.cm2 }},
		"other network":   {net2Key, "net2", func(f *receiveFixture) *mock.ChannelMembership { return f.cm2 }, func(f *receiveFixture) *mock.ChannelMembership { return f.cm }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReceiveFixture(t, []byte("bob"))

			_, tx, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, c.key))
			require.NoError(t, err)
			require.Equal(t, c.network, tx.Network())
			require.Equal(t, c.network, tx.FabricNetworkService().Name())
			require.Equal(t, 1, c.validateOn(f).IsValidCallCount(), "validated on the routed network")
			require.Zero(t, c.otherOn(f).IsValidCallCount(), "not validated on the other network")
		})
	}
}

// A transaction valid on the non-default network passes there even though the
// default network would reject its creator.
func TestNewTransactionFromBytes_ValidatesOnRoutedNetwork(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.cm.IsValidReturns(errors.New("unknown MSP"))

	_, tx, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, net2Key))
	require.NoError(t, err)
	require.Equal(t, "net2", tx.Network())
	require.Zero(t, f.metrics.count(security.KindInvalidTx))
}

func TestNewTransactionFromBytes_RejectsUnknownChannelKey(t *testing.T) {
	t.Parallel()
	for name, key := range map[string]*driver.ChannelKey{
		"genesis hash": {Platform: "generic", Channel: "ch1", GenesisHash: []byte("other")},
		"channel":      {Platform: "generic", Channel: "ch2", GenesisHash: []byte("genesis1")},
		"platform":     {Platform: "fabricx", Channel: "ch1", GenesisHash: []byte("genesis1")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReceiveFixture(t, []byte("bob"))

			_, _, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, key))
			require.ErrorContains(t, err, "no local network matches channel ["+key.Channel+"]")
			require.Zero(t, f.tm.NewTransactionFromBytesCallCount(), "nothing is decoded")
		})
	}
}

// A channel whose genesis hash cannot be read does not match; the transaction is
// rejected rather than built on another network.
func TestNewTransactionFromBytes_RejectsWhileGenesisHashUnknown(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.ch2.ChannelKeyReturns(nil, errors.New("ledger unavailable"))

	_, _, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, net2Key))
	require.ErrorContains(t, err, "no local network matches channel [ch1]")
	require.ErrorContains(t, err, "network [net2]: genesis hash unknown: ledger unavailable")
	require.Zero(t, f.tm.NewTransactionFromBytesCallCount(), "not built on the default network")
}

// Bytes without a key come from a node that does not send one and are decoded on
// the default network.
func TestNewTransactionFromBytes_LegacyBytesUseDefaultNetwork(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))

	_, tx, err := NewTransactionFromBytes(f.ctx, legacyRaw)
	require.NoError(t, err)
	require.Equal(t, "net1", tx.Network())
}

// The key names the network; a transaction on another channel of it is rejected.
func TestNewTransactionFromBytes_RejectsChannelMismatch(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.dtx2.ChannelReturns("ch2")

	_, _, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, net2Key))
	require.ErrorContains(t, err, "transaction channel [ch2] does not match its channel key [ch1]")
}

// A network that fails to load does not prevent routing to another one.
func TestNewTransactionFromBytes_SkipsFailingNetwork(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.fnsp.NamesReturns([]string{"broken", "net1", "net2"})

	_, tx, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, net2Key))
	require.NoError(t, err)
	require.Equal(t, "net2", tx.Network())
}

// Two local networks with the same channel key, as when one network is configured
// under two names: the default network wins if it is one of them.
func TestNewTransactionFromBytes_SameKeyPrefersDefault(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.ch2.ChannelKeyReturns(net1Key, nil)

	_, tx, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, net1Key))
	require.NoError(t, err)
	require.Equal(t, "net1", tx.Network())
}

// Without the default network among them, several matches are an error.
func TestNewTransactionFromBytes_SameKeyWithoutDefaultIsAmbiguous(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.ch2.ChannelKeyReturns(net1Key, nil)
	f.fnsp.DefaultNameReturns("other")

	_, _, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, net1Key))
	require.ErrorContains(t, err, "networks [net1 net2] all match channel [ch1] and none is the default network")
}

// An envelope is decoded on the network the caller names.
func TestNewTransactionFromEnvelopeBytesOn(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))

	for network, want := range map[string]string{"": "net1", "net2": "net2"} {
		_, tx, err := NewTransactionFromEnvelopeBytesOn(t.Context(), f.ctx, network, []byte("envelope"))
		require.NoError(t, err)
		require.Equal(t, want, tx.FabricNetworkService().Name())
	}
}

// Routing consults only a channel that a network of the key's platform has
// configured: a key naming another channel, or another platform, never makes the
// node open a channel, which would allocate its resources (the Fabric-x driver
// opens a channel of any name). The key matches what an unconfigured ch9 of net2
// would report, so only the configuration check keeps it from matching.
func TestNewTransactionFromBytes_ConsultsOnlyConfiguredChannels(t *testing.T) {
	t.Parallel()
	for name, key := range map[string]*driver.ChannelKey{
		"unconfigured channel": {Platform: "generic", Channel: "ch9", GenesisHash: []byte("genesis2")},
		"other platform":       {Platform: "fabricx", Channel: "ch1", GenesisHash: []byte("genesis2")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReceiveFixture(t, []byte("bob"))
			f.ch2.ChannelKeyReturns(key, nil)

			_, _, err := NewTransactionFromBytes(f.ctx, keyedRaw(t, key))
			require.ErrorContains(t, err, "no local network matches channel ["+key.Channel+"]")
			for _, n := range f.nets {
				require.Zero(t, n.ChannelCallCount(), "network [%s] opened a channel", n.Name())
			}
		})
	}
}

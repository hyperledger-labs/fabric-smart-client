/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
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

// receiveFixture is a view context whose network decodes every received
// transaction into dtx, with a session from caller.
type receiveFixture struct {
	ctx     *mock.Context
	dtx     *mock.Transaction
	tm      *mock.TransactionManager
	cm      *mock.ChannelMembership
	conf    *mock.ConfigService
	metrics *countingMetrics
}

func newReceiveFixture(t *testing.T, caller view.Identity) *receiveFixture {
	t.Helper()
	f := &receiveFixture{
		ctx:     &mock.Context{},
		dtx:     &mock.Transaction{},
		tm:      &mock.TransactionManager{},
		cm:      &mock.ChannelMembership{},
		conf:    &mock.ConfigService{},
		metrics: &countingMetrics{},
	}
	f.dtx.NetworkReturns("net1")
	f.dtx.ChannelReturns("ch1")
	f.dtx.IDReturns(testTxID)
	f.dtx.NonceReturns(testNonce)
	f.dtx.CreatorReturns(testCreator)
	f.tm.NewTransactionFromBytesReturns(f.dtx, nil)
	f.tm.ComputeTxIDStub = func(id *driver.TxIDComponents) string { return protoutil.ComputeTxID(id.Nonce, id.Creator) }
	f.conf.DriverNameReturns("generic")

	ch := &mock.Channel{}
	ch.NameReturns("ch1")
	ch.ChannelMembershipReturns(f.cm)
	fns := &mock.FabricNetworkService{}
	fns.NameReturns("net1")
	fns.ChannelReturns(ch, nil)
	fns.TransactionManagerReturns(f.tm)
	fns.ConfigServiceReturns(f.conf)
	fns.LocalMembershipReturns(&mock.LocalMembership{})
	fnsp := &mock.FabricNetworkServiceProvider{}
	fnsp.FabricNetworkServiceReturns(fns, nil)
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

func TestNewTransactionFromBytes_RejectsForgedTxID(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.dtx.IDReturns("forged")

	_, _, err := NewTransactionFromBytes(f.ctx, []byte("raw"))
	require.ErrorIs(t, err, ErrInvalidTransaction)
	require.InDelta(t, 1, f.metrics.count(security.KindInvalidTx), 0)
	// The error, which is also the reason the rejection reports, names the cause.
	require.Contains(t, err.Error(), `tx "forged": txID is not derived from nonce and creator`)
}

func TestNewTransactionFromBytes_Valid(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))

	_, tx, err := NewTransactionFromBytes(f.ctx, []byte("raw"))
	require.NoError(t, err)
	require.Equal(t, testTxID, tx.ID())
	require.Equal(t, testCreator, []byte(f.cm.IsValidArgsForCall(0)))
}

// A channel without configuration is a retryable condition, not a rejection.
func TestNewTransactionFromBytes_MSPNotInitializedNotCounted(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.cm.IsValidReturns(driver.ErrNotInitialized)

	_, _, err := NewTransactionFromBytes(f.ctx, []byte("raw"))
	require.ErrorIs(t, err, driver.ErrNotInitialized)
	require.Zero(t, f.metrics.count(security.KindInvalidTx))
}

// Fabric-x does not derive the txID from nonce and creator.
func TestNewTransactionFromBytes_FabricxSkipsTxIDCheck(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	f.conf.DriverNameReturns(fabricxDriverName)
	f.dtx.IDReturns("fabricx-txid")

	_, _, err := NewTransactionFromBytes(f.ctx, []byte("raw"))
	require.NoError(t, err)
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package security

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	fscview "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// fakeProvider records the counter options and the per-label-set counts.
type fakeProvider struct {
	metrics.Provider
	opts   metrics.CounterOpts
	mu     sync.Mutex
	counts map[string]float64
}

func (p *fakeProvider) NewCounter(o metrics.CounterOpts) metrics.Counter {
	p.opts = o
	p.counts = map[string]float64{}
	return &fakeCounter{p: p}
}

type fakeCounter struct {
	p      *fakeProvider
	labels []string
}

func (c *fakeCounter) With(labelValues ...string) metrics.Counter {
	return &fakeCounter{p: c.p, labels: append(append([]string{}, c.labels...), labelValues...)}
}

func (c *fakeCounter) Add(delta float64) {
	c.p.mu.Lock()
	defer c.p.mu.Unlock()
	key := ""
	for _, l := range c.labels {
		key += l + "|"
	}
	c.p.counts[key] += delta
}

func TestRejections_CountsByKind(t *testing.T) {
	t.Parallel()
	p := &fakeProvider{}
	r := NewRejections(p)

	r.Report(t.Context(), KindInvalidTx, []byte("peer"), "tx1", "bad")
	r.Report(t.Context(), KindInvalidTx, []byte("peer"), "tx1", "bad")

	require.Equal(t, "rejections", p.opts.Name)
	require.Empty(t, p.opts.Namespace)
	require.Equal(t, []string{"kind"}, p.opts.LabelNames)
	require.Equal(t, map[string]float64{"kind|invalid_tx|": 2}, p.counts)
}

func TestRejections_RateLimitsWarnLogs(t *testing.T) {
	t.Parallel()
	r := NewRejections(nil)
	now := time.Unix(1000, 0)
	r.now = func() time.Time { return now }
	warns := map[Kind]int{}
	var lastLine string
	r.warnf = func(_ context.Context, template string, args ...any) {
		warns[args[0].(Kind)]++
		lastLine = fmt.Sprintf(template, args...)
	}

	for range 100 {
		r.Report(t.Context(), KindInvalidTx, []byte("peer"), "tx1", "bad")
		r.Report(t.Context(), KindBindingRefused, []byte("peer"), "tx1\nforged", "bad")
		now = now.Add(time.Millisecond)
	}
	require.Equal(t, map[Kind]int{KindInvalidTx: 1, KindBindingRefused: 1}, warns)
	require.Equal(t, "security rejection [binding_refused] from peer ["+fscview.Identity("peer").String()+`] for tx "tx1\nforged": bad`, lastLine)

	now = now.Add(time.Second)
	r.Report(t.Context(), KindInvalidTx, []byte("peer"), "tx1", "bad")
	require.Equal(t, 2, warns[KindInvalidTx])
}

// The fallback is shared, so its rate limit holds across GetRejections calls.
// This is the only test that touches it.
func TestGetRejections_NoneRegisteredIsSharedNoop(t *testing.T) {
	t.Parallel()
	r := GetRejections(view.NewServiceProvider())
	require.NotNil(t, r)
	require.Same(t, r, GetRejections(view.NewServiceProvider()))

	warns := 0
	warnf := r.warnf
	t.Cleanup(func() { r.warnf = warnf })
	r.warnf = func(context.Context, string, ...any) { warns++ }
	GetRejections(view.NewServiceProvider()).Report(t.Context(), KindRecipientProof, []byte("peer"), "tx1", "bad")
	GetRejections(view.NewServiceProvider()).Report(t.Context(), KindRecipientProof, []byte("peer"), "tx1", "bad")
	require.Equal(t, 1, warns)
}

func TestGetRejections_ReturnsRegistered(t *testing.T) {
	t.Parallel()
	sp := view.NewServiceProvider()
	r := NewRejections(nil)
	require.NoError(t, sp.RegisterService(r))
	require.Same(t, r, GetRejections(sp))
}

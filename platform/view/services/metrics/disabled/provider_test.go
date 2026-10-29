/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package disabled_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

func TestProvider(t *testing.T) {
	t.Parallel()

	var p metrics.Provider = &disabled.Provider{}

	t.Run("NewCounter creates a no-op counter that doesn't blow up", func(t *testing.T) {
		t.Parallel()
		c := p.NewCounter(metrics.CounterOpts{})
		require.NotNil(t, c)

		c.Add(1)
		c.With("whatever").Add(2)
	})

	t.Run("NewGauge creates a no-op gauge that doesn't blow up", func(t *testing.T) {
		t.Parallel()
		g := p.NewGauge(metrics.GaugeOpts{})
		require.NotNil(t, g)

		g.Set(1)
		g.Add(1)
		g.With("whatever").Set(2)
		g.With("whatever").Add(2)
	})

	t.Run("NewHistogram creates a no-op histogram that doesn't blow up", func(t *testing.T) {
		t.Parallel()
		h := p.NewHistogram(metrics.HistogramOpts{})
		require.NotNil(t, h)

		h.Observe(1)
		h.With("whatever").Observe(2)
	})
}

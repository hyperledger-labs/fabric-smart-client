/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"strings"
	"testing"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	fscprom "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/prometheus"
)

// TestMetricNamesMatchCatalog pins the exported names of the delivery counters.
//
// The Prometheus provider derives a metric's namespace and subsystem from the
// package that calls NewCounter, so the exported name depends on where NewMetrics
// lives rather than on anything stated at the call site. Moving it would silently
// rename both counters and break every dashboard and alert built on them, so the
// names are asserted here against the ones written down in
// docs/platform/view/services/monitoring_metrics.md.
//
//nolint:paralleltest // registers into and gathers from the default Prometheus registry, which is process-global: running in parallel with another test that registers metrics makes the gather racy
func TestMetricNamesMatchCatalog(t *testing.T) {
	m := NewMetrics(&fscprom.Provider{})
	m.CommitRetries.Add(1)
	m.CommitFailures.With(failureClassLabel, classDegrade.String()).Add(1)

	families, err := prom.DefaultGatherer.Gather()
	require.NoError(t, err)

	got := make(map[string]bool, len(families))
	for _, f := range families {
		got[f.GetName()] = true
	}

	for _, want := range []string{
		"fsc_fabric_core_generic_delivery_commit_retries",
		"fsc_fabric_core_generic_delivery_commit_failures",
	} {
		require.True(t, got[want],
			"expected metric %q to be registered; registered delivery metrics: %v",
			want, deliveryNames(got))
	}
}

// deliveryNames narrows the gathered names to this package's own, so a failure
// message shows the near misses rather than every metric in the process.
func deliveryNames(m map[string]bool) string {
	names := make([]string, 0, len(m))
	for k := range m {
		if strings.Contains(k, "delivery") {
			names = append(names, k)
		}
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

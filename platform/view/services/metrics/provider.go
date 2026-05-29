/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

var key = reflect.TypeFor[*Provider]()

// A Provider is an abstraction for a metrics provider. It is a factory for
// Counter, Gauge, and Histogram meters.
type Provider interface {
	// NewCounter creates a new instance of a Counter.
	NewCounter(CounterOpts) Counter
	// NewGauge creates a new instance of a Gauge.
	NewGauge(GaugeOpts) Gauge
	// NewHistogram creates a new instance of a Histogram.
	NewHistogram(HistogramOpts) Histogram
}

// A Counter represents a monotonically increasing value.
type Counter interface {
	// With is used to provide label values when updating a Counter. This must be
	// used to provide values for all LabelNames provided to CounterOpts.
	With(labelValues ...string) Counter

	// Add increments a counter value.
	Add(delta float64)
}

// CounterOpts is used to provide basic information about a counter to the
// metrics subsystem.
type CounterOpts struct {
	// Namespace, Subsystem, and Name are components of the fully-qualified name
	// of the Metric. The fully-qualified aneme is created by joining these
	// components with an appropriate separator. Only Name is mandatory, the
	// others merely help structuring the name.
	Namespace string
	Subsystem string
	Name      string

	// Help provides information about this metric.
	Help string

	// LabelNames provides the names of the labels that can be attached to this
	// metric. When a metric is recorded, label values must be provided for each
	// of these label names.
	LabelNames []string

	// LabelHelp provides help information for labels. When set, this information
	// will be used to populate the documentation.
	LabelHelp map[string]string
}

// A Gauge is a meter that expresses the current value of some metric.
type Gauge interface {
	// With is used to provide label values when recording a Gauge value. This
	// must be used to provide values for all LabelNames provided to GaugeOpts.
	With(labelValues ...string) Gauge

	// Add increments a Gauge value.
	Add(delta float64) // TODO: consider removing

	// Set is used to update the current value associated with a Gauge.
	Set(value float64)
}

// GaugeOpts is used to provide basic information about a gauge to the
// metrics subsystem.
type GaugeOpts struct {
	// Namespace, Subsystem, and Name are components of the fully-qualified name
	// of the Metric. The fully-qualified aneme is created by joining these
	// components with an appropriate separator. Only Name is mandatory, the
	// others merely help structuring the name.
	Namespace string
	Subsystem string
	Name      string

	// Help provides information about this metric.
	Help string

	// LabelNames provides the names of the labels that can be attached to this
	// metric. When a metric is recorded, label values must be provided for each
	// of these label names.
	LabelNames []string

	// LabelHelp provides help information for labels. When set, this information
	// will be used to populate the documentation.
	LabelHelp map[string]string
}

// A Histogram is a meter that records an observed value into quantized
// buckets.
type Histogram interface {
	// With is used to provide label values when recording a Histogram
	// observation. This must be used to provide values for all LabelNames
	// provided to HistogramOpts.
	With(labelValues ...string) Histogram
	Observe(value float64)
}

// HistogramOpts is used to provide basic information about a histogram to the
// metrics subsystem.
type HistogramOpts struct {
	// Namespace, Subsystem, and Name are components of the fully-qualified name
	// of the Metric. The fully-qualified aneme is created by joining these
	// components with an appropriate separator. Only Name is mandatory, the
	// others merely help structuring the name.
	Namespace string
	Subsystem string
	Name      string

	// Help provides information about this metric.
	Help string

	// Buckets can be used to provide the bucket boundaries for Prometheus. When
	// omitted, the default Prometheus bucket values are used.
	Buckets []float64

	// NativeHistogramBucketFactor determines the resolution of native histogram
	// buckets. A value of 1.1 (schema=3) means each bucket boundary is ~9% wider
	// than the previous. Setting this to a value > 1 enables native histogram
	// collection alongside classic buckets (dual mode). When zero, only classic
	// buckets are used.
	NativeHistogramBucketFactor float64

	// NativeHistogramMaxBucketNumber is the maximum number of populated sparse
	// buckets allowed. If this limit is exceeded during observation, the
	// histogram resolution is reduced (buckets are merged) to stay within the
	// limit. A value of 0 means no limit.
	NativeHistogramMaxBucketNumber uint32

	// NativeHistogramZeroThreshold defines the width of the zero bucket.
	// Observations in the range [-ZeroThreshold, +ZeroThreshold] are counted in
	// a dedicated zero bucket. A value of 0 means the zero bucket only contains
	// observations that are exactly zero.
	NativeHistogramZeroThreshold float64

	// LabelNames provides the names of the labels that can be attached to this
	// metric. When a metric is recorded, label values must be provided for each
	// of these label names.
	LabelNames []string

	// LabelHelp provides help information for labels. When set, this information
	// will be used to populate the documentation.
	LabelHelp map[string]string
}

// GetProvider returns the metrics provider registered in the service provider passed in.
func GetProvider(sp services.Provider) Provider {
	s, err := sp.GetService(key)
	if err != nil {
		panic(err)
	}
	return s.(Provider)
}

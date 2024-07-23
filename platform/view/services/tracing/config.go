/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	namespaceKey  = "namespace"
	labelNamesKey = "label_names"
	nodeNameKey   = "node_name"
)

type LabelName = string

type MetricsOpts struct {
	Namespace  string
	LabelNames []LabelName
}

func WithMetricsOpts(o MetricsOpts) trace.TracerOption {
	set := attribute.NewSet(
		attribute.String(namespaceKey, o.Namespace),
		attribute.StringSlice(labelNamesKey, o.LabelNames),
	)
	return trace.WithInstrumentationAttributes(set.ToSlice()...)
}

func extractMetricsOpts(attrs attribute.Set) MetricsOpts {
	o := MetricsOpts{}
	if val, ok := attrs.Value(namespaceKey); ok {
		o.Namespace = val.AsString()
	}
	if val, ok := attrs.Value(nodeNameKey); ok {
		o.Namespace = val.AsString() + "_" + o.Namespace
	}
	if val, ok := attrs.Value(labelNamesKey); ok {
		o.LabelNames = val.AsStringSlice()
	}
	return o
}

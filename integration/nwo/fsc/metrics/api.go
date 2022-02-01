/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dmetrics

import (
	"fmt"
	"time"

	"github.com/armon/go-metrics"
)

type SinkInput struct {
	Key []string
	Val float32
}

type Shim interface {
	metrics.MetricSink
}

type DistributedEvent interface {
	AddGauge(func(Shim, ...Event) (remaining []Event))

	AddKeyEmission(func(Shim, ...Event) (remaining []Event))

	AddCounterInc(func(Shim, ...Event) (remaining []Event))

	AddSample(func(Shim, ...Event) (remaining []Event))
}

type RawEvent struct {
	Timestamp time.Time
	Payload   string
}

type Event struct {
	Raw       *RawEvent
	Type      string
	Keys      []string
	Value     float32
	Host      string
	Timestamp time.Time
}

func (e *Event) String() string {
	return fmt.Sprintf("Type: %s, Value: %f, Keys: %v, Host: %s, time: %v", e.Type, e.Value, e.Keys, e.Host, e.Raw.Timestamp)
}

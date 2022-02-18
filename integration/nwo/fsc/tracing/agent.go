/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dmetrics

import (
	"strconv"
	"strings"
	"time"

	"github.com/armon/go-metrics"
)

type Agent struct {
	sink     *metrics.StatsdSink
	hostname string
}

type Host string
type StatsDSink string

func NewAgent(host Host, statsEndpoint StatsDSink) *Agent {
	sink, _ := metrics.NewStatsdSink(string(statsEndpoint))
	return &Agent{sink: sink, hostname: strings.Replace(string(host), ".", "!", -1)}
}

func (a *Agent) EmitKey(val float32, event ...string) {
	now := time.Now().UnixNano()
	events := append([]string{a.hostname}, event...)
	events = append(events, strconv.FormatInt(now, 10))
	a.sink.EmitKey(events, val)
}

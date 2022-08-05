/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"strconv"
	"strings"
	"time"

	"github.com/armon/go-metrics"
	"github.com/pkg/errors"
)

type Host string
type StatsDSink string

type StatsdAgent struct {
	sink     *metrics.StatsdSink
	hostname string
}

func NewStatsdAgent(host Host, statsEndpoint StatsDSink) (*StatsdAgent, error) {
	sink, err := metrics.NewStatsdSink(string(statsEndpoint))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create statsd sink at [%s]", statsEndpoint)
	}
	return &StatsdAgent{sink: sink, hostname: strings.Replace(string(host), ".", "!", -1)}, nil
}

func (a *StatsdAgent) EmitKey(val float32, event ...string) {
	// logger.Debugf("EmitKey: [%s]", event)
	now := time.Now().UnixNano()
	events := append([]string{a.hostname}, event...)
	events = append(events, strconv.FormatInt(now, 10))
	a.sink.EmitKey(events, val)
}

type NullAgent struct{}

func NewNullAgent() *NullAgent {
	return &NullAgent{}
}

func (a *NullAgent) EmitKey(val float32, event ...string) {
	// Do nothing
}

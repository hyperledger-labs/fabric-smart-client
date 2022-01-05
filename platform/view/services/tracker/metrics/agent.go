/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"strconv"
	"strings"
	"time"

	"github.com/armon/go-metrics"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("view.tracker.metrics")

type Agent struct {
	sink     *metrics.StatsdSink
	hostname string
}

type Host string
type StatsDSink string

func NewStatsdAgent(host Host, statsEndpoint StatsDSink) (*Agent, error) {
	sink, err := metrics.NewStatsdSink(string(statsEndpoint))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create statsd sink at [%s]", statsEndpoint)
	}
	return &Agent{sink: sink, hostname: strings.Replace(string(host), ".", "!", -1)}, nil
}

func (a *Agent) EmitKey(val float32, event ...string) {
	// logger.Debugf("EmitKey: [%s]", event)
	now := time.Now().UnixNano()
	events := append([]string{a.hostname}, event...)
	events = append(events, strconv.FormatInt(now, 10))
	a.sink.EmitKey(events, val)
}

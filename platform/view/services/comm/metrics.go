/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

const (
	PeerId     = "peer"
	ProtocolId = "protocol"
)

var (
	bytesSent = metrics.CounterOpts{
		Namespace:    "comm",
		Name:         "bytes_sent",
		Help:         "The amount of data sent.",
		LabelNames:   []string{PeerId, ProtocolId},
		StatsdFormat: "%{#fqname}.%{" + PeerId + "}.%{" + ProtocolId + "}",
	}
	bytesReceived = metrics.CounterOpts{
		Namespace:    "comm",
		Name:         "bytes_received",
		Help:         "The amount of data received.",
		LabelNames:   []string{PeerId, ProtocolId},
		StatsdFormat: "%{#fqname}.%{" + PeerId + "}.%{" + ProtocolId + "}",
	}
)

type Metrics struct {
	BytesSent     metrics.Counter
	BytesReceived metrics.Counter
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		BytesSent:     p.NewCounter(bytesSent),
		BytesReceived: p.NewCounter(bytesReceived),
	}
}

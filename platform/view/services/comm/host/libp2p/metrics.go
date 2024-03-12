/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

const (
	PeerId     = "peer"
	ProtocolId = "protocol"
)

var (
	bytesSent = metrics2.CounterOpts{
		Namespace:    "comm",
		Name:         "bytes_sent",
		Help:         "The amount of data sent.",
		LabelNames:   []string{PeerId, ProtocolId},
		StatsdFormat: "%{#fqname}.%{" + PeerId + "}.%{" + ProtocolId + "}",
	}
	bytesReceived = metrics2.CounterOpts{
		Namespace:    "comm",
		Name:         "bytes_received",
		Help:         "The amount of data received.",
		LabelNames:   []string{PeerId, ProtocolId},
		StatsdFormat: "%{#fqname}.%{" + PeerId + "}.%{" + ProtocolId + "}",
	}
)

type metrics struct {
	BytesSent     metrics2.Counter
	BytesReceived metrics2.Counter
}

func newMetrics(p metrics2.Provider) *metrics {
	return &metrics{
		BytesSent:     p.NewCounter(bytesSent),
		BytesReceived: p.NewCounter(bytesReceived),
	}
}

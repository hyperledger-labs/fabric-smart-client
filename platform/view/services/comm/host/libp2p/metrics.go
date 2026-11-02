/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import viewmetrics "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

const (
	//nolint:revive // var-naming: renaming this exported const is an API break; see follow-up
	PeerId = "peer"
	//nolint:revive // var-naming: renaming this exported const is an API break; see follow-up
	ProtocolId = "protocol"
)

var (
	bytesSent = viewmetrics.CounterOpts{
		Name:       "bytes_sent",
		Help:       "The amount of data sent.",
		LabelNames: []string{PeerId, ProtocolId},
	}
	bytesReceived = viewmetrics.CounterOpts{
		Name:       "bytes_received",
		Help:       "The amount of data received.",
		LabelNames: []string{PeerId, ProtocolId},
	}
	rejectedStreams = viewmetrics.CounterOpts{
		Name:       "rejected_streams",
		Help:       "The number of streams refused, by reason.",
		LabelNames: []string{reasonLabel},
	}
)

const (
	reasonLabel = "reason"
	// versionReason labels streams refused because of the peer's preamble.
	versionReason = "version"
)

type metrics struct {
	BytesSent       viewmetrics.Counter
	BytesReceived   viewmetrics.Counter
	RejectedStreams viewmetrics.Counter
}

func newMetrics(p viewmetrics.Provider) *metrics {
	return &metrics{
		BytesSent:       p.NewCounter(bytesSent),
		BytesReceived:   p.NewCounter(bytesReceived),
		RejectedStreams: p.NewCounter(rejectedStreams),
	}
}

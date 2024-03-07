/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

type metricsReporter struct {
	m *Metrics
}

func newReporter(m *Metrics) *metricsReporter {
	logger.Infof("Initialized bandwidth reporter.\n")
	return &metricsReporter{m}
}

func (r *metricsReporter) LogSentMessageStream(size int64, proto protocol.ID, p peer.ID) {
	r.m.BytesSent.With(PeerId, p.String(), ProtocolId, string(proto)).Add(float64(size))
}

func (r *metricsReporter) LogRecvMessageStream(size int64, proto protocol.ID, p peer.ID) {
	r.m.BytesReceived.With(PeerId, p.String(), ProtocolId, string(proto)).Add(float64(size))
}

func (r *metricsReporter) LogSentMessage(int64)                              {}
func (r *metricsReporter) LogRecvMessage(int64)                              {}
func (r *metricsReporter) GetBandwidthForPeer(peer.ID) metrics.Stats         { return metrics.Stats{} }
func (r *metricsReporter) GetBandwidthForProtocol(protocol.ID) metrics.Stats { return metrics.Stats{} }
func (r *metricsReporter) GetBandwidthTotals() metrics.Stats                 { return metrics.Stats{} }
func (r *metricsReporter) GetBandwidthByPeer() map[peer.ID]metrics.Stats {
	return map[peer.ID]metrics.Stats{}
}

func (r *metricsReporter) GetBandwidthByProtocol() map[protocol.ID]metrics.Stats {
	return map[protocol.ID]metrics.Stats{}
}

package comm

import (
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

type MetricsReporter struct {
	m *Metrics
}

func NewReporter(m *Metrics) *MetricsReporter {
	logger.Infof("Initialized bandwidth reporter.\n")
	return &MetricsReporter{m}
}

func (r *MetricsReporter) LogSentMessageStream(size int64, proto protocol.ID, p peer.ID) {
	r.m.BytesSent.With(PeerId, p.String(), ProtocolId, string(proto)).Add(float64(size))
}
func (r *MetricsReporter) LogRecvMessageStream(size int64, proto protocol.ID, p peer.ID) {
	r.m.BytesReceived.With(PeerId, p.String(), ProtocolId, string(proto)).Add(float64(size))
}
func (r *MetricsReporter) LogSentMessage(int64)                              {}
func (r *MetricsReporter) LogRecvMessage(int64)                              {}
func (r *MetricsReporter) GetBandwidthForPeer(peer.ID) metrics.Stats         { return metrics.Stats{} }
func (r *MetricsReporter) GetBandwidthForProtocol(protocol.ID) metrics.Stats { return metrics.Stats{} }
func (r *MetricsReporter) GetBandwidthTotals() metrics.Stats                 { return metrics.Stats{} }
func (r *MetricsReporter) GetBandwidthByPeer() map[peer.ID]metrics.Stats {
	return map[peer.ID]metrics.Stats{}
}
func (r *MetricsReporter) GetBandwidthByProtocol() map[protocol.ID]metrics.Stats {
	return map[protocol.ID]metrics.Stats{}
}

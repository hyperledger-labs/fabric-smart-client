/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"context"
	"net"
	"net/http"
	"time"

	kitstatsd "github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/statsd"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/statsd/goruntime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

//go:generate counterfeiter -o fakes/logger.go -fake-name Logger . Logger

type Logger interface {
	Debugf(template string, args ...interface{})
	Info(...interface{})
	Warn(args ...interface{})
	Warnf(template string, args ...interface{})
}

type Statsd struct {
	Network       string
	Address       string
	WriteInterval time.Duration
	Prefix        string
}

type MetricsOptions struct {
	Provider string
	Statsd   *Statsd
	TLS      bool
}

type TLS struct {
	Enabled bool
}

type Options struct {
	TLS     TLS
	Metrics MetricsOptions
	Version string
	Logger  Logger
}

type Server interface {
	RegisterHandler(s string, handler http.Handler, secure bool)
}

type System struct {
	metrics.Provider

	Server          Server
	logger          OperationsLogger
	options         Options
	statsd          *kitstatsd.Statsd
	collectorTicker *time.Ticker
	sendTicker      *time.Ticker
	versionGauge    metrics.Gauge
}

func NewOperationSystem(server Server, l OperationsLogger, metricsProvider metrics.Provider, o *Options) (*System, error) {
	system := &System{
		Server:  server,
		logger:  l,
		options: *o,
	}
	system.initializeLoggingHandler(o.TLS.Enabled)
	if err := system.initializeMetricsProvider(metricsProvider, o.Metrics); err != nil {
		return nil, errors.Wrap(err, "failed to initialize metrics provider")
	}

	return system, nil
}

func (s *System) Start() error {
	err := s.startMetricsTickers(s.options.Metrics.Statsd)
	if err != nil {
		return err
	}

	s.versionGauge.With("version", s.options.Version).Set(1)
	return nil
}

func (s *System) Stop() error {
	if s.collectorTicker != nil {
		s.collectorTicker.Stop()
		s.collectorTicker = nil
	}
	if s.sendTicker != nil {
		s.sendTicker.Stop()
		s.sendTicker = nil
	}
	return nil
}

func (s *System) Log(keyvals ...interface{}) error {
	return s.logger.Log(keyvals)
}

func (s *System) initializeMetricsProvider(provider metrics.Provider, m MetricsOptions) error {
	s.logger.Debugf("Initializing metrics provider: [%s]", m.Provider)
	s.Provider = provider
	switch m.Provider {
	case "statsd":
		s.statsd = provider.(*statsd.Provider).Statsd
	case "prometheus":
		// swagger:operation GET /metrics operations metrics
		// ---
		// responses:
		//     '200':
		//        description: Ok.
		s.Server.RegisterHandler("/metrics", promhttp.Handler(), m.TLS)
	case "":
		s.logger.Info("metrics disabled")
	default:
		s.logger.Warnf("unknown provider type: %s; metrics disabled", m.Provider)
	}
	s.versionGauge = versionGauge(s.Provider)
	return nil
}

func (s *System) initializeLoggingHandler(tlsEnabled bool) {
	// swagger:operation GET /logspec operations logspecget
	// ---
	// summary: Retrieves the active logging spec for a peer or orderer.
	// responses:
	//     '200':
	//        description: Ok.

	// swagger:operation PUT /logspec operations logspecput
	// ---
	// summary: Updates the active logging spec for a peer or orderer.
	//
	// parameters:
	// - name: payload
	//   in: formData
	//   type: string
	//   description: The payload must consist of a single attribute named spec.
	//   required: true
	// responses:
	//     '204':
	//        description: No content.
	//     '400':
	//        description: Bad request.
	// consumes:
	//   - multipart/form-data
	s.Server.RegisterHandler("/logspec", logging.NewSpecHandler(), tlsEnabled)
}

func (s *System) startMetricsTickers(m *Statsd) error {
	if s.statsd != nil {
		network := m.Network
		address := m.Address
		c, err := net.Dial(network, address)
		if err != nil {
			return err
		}
		utils.IgnoreErrorFunc(c.Close)

		writeInterval := m.WriteInterval

		s.collectorTicker = time.NewTicker(writeInterval / 2)
		goCollector := goruntime.NewCollector(s.Provider)
		go goCollector.CollectAndPublish(s.collectorTicker.C)

		s.sendTicker = time.NewTicker(writeInterval)
		go s.statsd.SendLoop(context.Background(), s.sendTicker.C, network, address)
	}

	return nil
}

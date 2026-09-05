/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

// Logger is the subset of logging this package needs.
type Logger interface {
	Debugf(template string, args ...any)
	Info(...any)
	Warn(args ...any)
	Warnf(template string, args ...any)
}

// MetricsOptions selects the metrics backend.
type MetricsOptions struct {
	Provider string
}

// Options configures the operations system.
type Options struct {
	Metrics MetricsOptions
	Version string
	Logger  Logger
	// RequireClientCert makes the operations endpoints demand a verified client certificate.
	// It follows the listener's own client authentication rather than being configured
	// separately: an endpoint cannot require a certificate a listener never asks for.
	RequireClientCert bool
}

// Server is the listener the operations endpoints are registered on.
type Server interface {
	RegisterHandler(s string, handler http.Handler, secure bool)
}

// System serves the node's operations endpoints and owns its metrics provider.
type System struct {
	metrics.Provider

	Server          Server
	logger          OperationsLogger
	options         Options
	collectorTicker *time.Ticker
	sendTicker      *time.Ticker
	versionGauge    metrics.Gauge
}

// NewOperationSystem registers the operations endpoints on server and initialises the
// metrics provider. It returns an error if the configured provider cannot be initialised;
// an unknown provider disables metrics with a warning rather than failing.
func NewOperationSystem(server Server, l OperationsLogger, metricsProvider metrics.Provider, o *Options) (*System, error) {
	system := &System{
		Server:  server,
		logger:  l,
		options: *o,
	}
	system.initializeLoggingHandler(o.RequireClientCert)
	if err := system.initializeMetricsProvider(metricsProvider, o.Metrics); err != nil {
		return nil, errors.Wrap(err, "failed to initialize metrics provider")
	}

	return system, nil
}

// Start publishes the version gauge. The endpoints are already registered by this point.
func (s *System) Start() error {
	s.versionGauge.With("version", s.options.Version).Set(1)
	return nil
}

// Stop halts the collector tickers. It is safe to call when Start was never called.
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

func (s *System) initializeMetricsProvider(provider metrics.Provider, m MetricsOptions) error {
	s.logger.Debugf("Initializing metrics provider: [%s]", m.Provider)
	s.Provider = provider
	switch m.Provider {
	case "prometheus":
		// swagger:operation GET /metrics operations metrics
		// ---
		// responses:
		//     '200':
		//        description: Ok.
		s.Server.RegisterHandler("/metrics", promhttp.Handler(), s.options.RequireClientCert)
	case "":
		s.logger.Info("metrics disabled")
	default:
		s.logger.Warnf("unknown provider type: %s; metrics disabled", m.Provider)
	}
	s.versionGauge = versionGauge(s.Provider)
	return nil
}

func (s *System) initializeLoggingHandler(requireClientCert bool) {
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
	s.Server.RegisterHandler("/logspec", logging.NewSpecHandler(), requireClientCert)
}

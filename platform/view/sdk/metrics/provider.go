/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing/optl"
)

var logger = flogging.MustGetLogger("view-sdk")

func NewProvider(opts *operations.Options, logger log.Logger) metrics.Provider {
	return operations.NewMetricsProvider(opts.Metrics, logger)
}

func NewTracer(provider *tracing.Provider) tracing.Tracer {
	return provider.GetTracer()
}

func NewTracingProvider(confService driver.ConfigService) (*tracing.Provider, error) {
	switch providerType := confService.GetString("fsc.tracing.provider"); providerType {
	case "", "none":
		logger.Infof("Tracing disabled")
		return tracing.NewProvider(disabled.New()), nil
	case "optl":
		logger.Infof("Tracing enabled: optl")
		address := confService.GetString("fsc.tracing.optl.address")
		if len(address) == 0 {
			address = "localhost:4319"
			logger.Infof("tracing server address not set, using default: [%s]", address)
		}
		tp := optl.LaunchOptl(address, context.Background())
		return tracing.NewProvider(optl.NewLatencyTracer(tp, optl.LatencyTracerOpts{Name: "FSC-Tracing"})), nil
	default:
		return nil, fmt.Errorf("unknown tracing provider: %s", providerType)
	}
}

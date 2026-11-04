/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package libp2p installs the libp2p host on FSC nodes built by nwo. The rest of nwo does not
// link it, so only a topology that imports this package builds libp2p.
package libp2p

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/provider"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// SDK wraps another SDK and makes its nodes use the libp2p host when fsc.p2p.type is libp2p, and
// the default host provider otherwise.
type SDK struct {
	common.SDK
}

// NewSDK returns an SDK that wraps the view SDK.
func NewSDK(registry services.Registry) *SDK {
	return NewFrom(viewsdk.NewSDK(registry))
}

// NewFrom returns an SDK that wraps sdk.
func NewFrom(sdk common.SDK) *SDK {
	return &SDK{SDK: sdk}
}

// Install decorates the host provider with CustomHostProvider and installs the wrapped SDK.
func (p *SDK) Install() error {
	if err := p.Container().Decorate(CustomHostProvider); err != nil {
		return err
	}
	return p.SDK.Install()
}

// CustomHostProvider extends the default host provider with libp2p support.
func CustomHostProvider(config driver.ConfigService, endpointService *endpoint.Service, metricsProvider metrics.Provider, tracerProvider tracing.Provider) (host.GeneratorProvider, error) {
	p2pCommType := strings.ToLower(config.GetString("fsc.p2p.type"))
	switch p2pCommType {
	case libp2p.P2PCommunicationType:
		if err := endpointService.AddPublicKeyExtractor(&comm.PKExtractor{}); err != nil {
			return nil, err
		}
		endpointService.SetPublicKeyIDSynthesizer(&libp2p.PKIDSynthesizer{})
		return libp2p.NewHostGeneratorProvider(libp2p.NewConfig(config), metricsProvider), nil
	default:
		return provider.NewHostProvider(config, endpointService, metricsProvider, tracerProvider)
	}
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package relay

import (
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

type Provider interface {
	Relay(fns *fabric.NetworkService) *weaver.Relay
}

type SDK struct {
	dig2.SDK
}

func NewSDK(registry node.Registry) *SDK {
	return &SDK{SDK: fabricsdk.NewSDK(registry)}
}

func (p *SDK) Install() error {
	if err := errors.Join(
		p.Container().Provide(weaver.NewProvider, dig.As(new(Provider))),
	); err != nil {
		return err
	}
	if err := p.SDK.Install(); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[Provider](p.Container()),
		digutils.Register[trace.TracerProvider](p.Container()),
		digutils.Register[driver.EndpointService](p.Container()),
		digutils.Register[view3.IdentityProvider](p.Container()),
		digutils.Register[node.ViewManager](p.Container()), // Need to add it as a field in the node
		digutils.Register[id.SigService](p.Container()),
		digutils.Register[*fabric.NetworkServiceProvider](p.Container()), // GetFabricNetworkService is used by many components
	)
}

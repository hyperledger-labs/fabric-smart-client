/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tokenx

import (
	"context"
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/tokenx/api"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	viewgrpc "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	webserver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"go.uber.org/dig"
)

// SDK is the Token Management SDK that extends the FabricX SDK
type SDK struct {
	common.SDK
}

// NewSDK creates a new Token Management SDK from a services registry
func NewSDK(registry services.Registry) *SDK {
	return NewFrom(fabricx.NewSDK(registry))
}

// NewFrom creates a Token Management SDK from an existing SDK
func NewFrom(sdk common.SDK) *SDK {
	return &SDK{SDK: sdk}
}

// Install registers all required services for the token management application
func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
	)
}

type localViewCaller struct {
	viewManager viewgrpc.ViewManager
}

func (c *localViewCaller) CallView(ctx context.Context, vid string, input []byte) (interface{}, error) {
	v, err := c.viewManager.NewView(vid, input)
	if err != nil {
		return nil, err
	}
	return c.viewManager.InitiateView(v, ctx)
}

func (p *SDK) Start(ctx context.Context) error {
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}

	return p.Container().Invoke(func(in struct {
		dig.In
		Handler         *webserver.HttpHandler
		ViewManager     viewgrpc.ViewManager
		EndpointService *endpoint.Service
	}) error {
		vc := &localViewCaller{viewManager: in.ViewManager}
		tokenAPI := api.NewTokenAPI(vc, in.EndpointService)
		tokenAPI.RegisterHandlers(in.Handler)
		return nil
	})
}

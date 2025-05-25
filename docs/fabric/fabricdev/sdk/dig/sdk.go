/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/sdk/fabricdev"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"go.uber.org/dig"
)

// SDK installs a new fabric driver
type SDK struct {
	dig2.SDK
}

func NewSDK(registry node.Registry) *SDK {
	return NewFrom(fabricsdk.NewSDK(registry))
}

func NewFrom(sdk dig2.SDK) *SDK {
	return &SDK{SDK: sdk}
}

func (p *SDK) FabricEnabled() bool {
	return p.ConfigService().GetBool("fabric.enabled")
}

func (p *SDK) Install(ctx context.Context) error {
	if !p.FabricEnabled() {
		return p.SDK.Install(ctx)
	}
	err := errors.Join(
		// Register the new fabric platform driver
		p.Container().Provide(fabricdev.NewDriver, dig.Group("fabric-platform-drivers")),
	)
	if err != nil {
		return err
	}

	if err := p.SDK.Install(ctx); err != nil {
		return err
	}

	return nil
}

func (p *SDK) Start(ctx context.Context) error {
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}
	return nil
}

func (p *SDK) PostStart(ctx context.Context) error {
	if err := p.SDK.PostStart(ctx); err != nil {
		return err
	}
	return nil
}

/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dig

import (
	"context"
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"go.uber.org/dig"
)

type SDK interface {
	api.SDK
	PostStart(ctx context.Context) error
	Stop() error
	Container() *dig.Container
	ConfigService() driver.ConfigService
}

type BaseSDK struct {
	C      *dig.Container
	Config *view.ConfigService
}

func (s *BaseSDK) PostStart(context.Context) error { return nil }

func (s *BaseSDK) Stop() error { return nil }

func (s *BaseSDK) Container() *dig.Container { return s.C }

func (s *BaseSDK) ConfigService() driver.ConfigService { return s.Config }

func NewSDK(registry node.Registry) *BaseSDK {
	return NewSDKWithContainer(dig.New(), registry)
}

func NewSDKWithContainer(c *dig.Container, registry node.Registry) *BaseSDK {
	sdk := &BaseSDK{
		C:      c,
		Config: view.GetConfigService(registry),
	}
	err := errors.Join(
		sdk.C.Provide(func() node.Registry { return registry }),
	)
	if err != nil {
		panic(err)
	}
	return sdk
}

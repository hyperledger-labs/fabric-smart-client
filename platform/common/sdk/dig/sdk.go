/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dig

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"go.uber.org/dig"
)

type (
	ProvideOption  any
	InvokeOption   = dig.InvokeOption
	DecorateOption = dig.DecorateOption
)

type Container interface {
	Provide(constructor interface{}, opts ...ProvideOption) error
	Invoke(function interface{}, opts ...InvokeOption) error
	Decorate(decorator interface{}, opts ...DecorateOption) error
	Visualize() string
}

type SDK interface {
	node.SDK
	PostStart(ctx context.Context) error
	Stop() error
	Container() Container
	ConfigService() driver.ConfigService
}

func NewBaseSDK(c Container, cfg driver.ConfigService) *BaseSDK {
	return &BaseSDK{c: c, cfg: cfg}
}

type BaseSDK struct {
	c   Container
	cfg driver.ConfigService
}

func (s *BaseSDK) Install() error { return nil }

func (s *BaseSDK) Start(context.Context) error { return nil }

func (s *BaseSDK) ConfigService() driver.ConfigService { return s.cfg }

func (s *BaseSDK) Container() Container { return s.c }

func (s *BaseSDK) PostStart(context.Context) error { return nil }

func (s *BaseSDK) Stop() error { return nil }

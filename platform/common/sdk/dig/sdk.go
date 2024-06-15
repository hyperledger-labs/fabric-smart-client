/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dig

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
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

type BaseSDK struct{}

func (s *BaseSDK) PostStart(context.Context) error { return nil }

func (s *BaseSDK) Stop() error { return nil }

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"go.uber.org/dig"
)

func NewContainer(opts ...dig.Option) *baseContainer {
	return &baseContainer{Container: dig.New(opts...)}
}

type baseContainer struct{ *dig.Container }

func (c *baseContainer) Provide(constructor any, options ...common.ProvideOption) error {
	opts := make([]dig.ProvideOption, len(options))
	for i, option := range options {
		opt, ok := option.(dig.ProvideOption)
		if !ok {
			return errors.New("invalid option")
		}
		opts[i] = opt
	}
	return c.Container.Provide(constructor, opts...)
}

func (c *baseContainer) Visualize() string {
	return digutils.Visualize(c.Container)
}

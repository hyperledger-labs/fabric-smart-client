/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vfsdk

import (
	"context"
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	errors2 "github.com/pkg/errors"
	"go.uber.org/dig"
)

type factoryRegisterer interface {
	RegisterFactory(id string, factory driver.Factory) error
	RegisterResponderFactory(factory driver.Factory, initiatedBy interface{}) error
}

type SDK struct {
	dig2.SDK
}

func NewSDK(registry node.Registry) *SDK {
	return NewFrom(viewsdk.NewSDKFromContainer(NewContainer(), registry))
}

func NewFrom(sdk dig2.SDK) *SDK {
	return &SDK{SDK: sdk}
}

func (p *SDK) Install() error {
	err := errors.Join(
		p.Container().Provide(digutils.Identity[*registry.ViewProvider](), dig.As(new(factoryRegisterer))),
	)
	if err != nil {
		return err
	}

	return p.SDK.Install()
}

func (p *SDK) Start(ctx context.Context) error {
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}
	return p.Container().Invoke(func(in struct {
		dig.In
		ViewProvider   factoryRegisterer
		FactoryEntries []*factoryEntry `group:"view-factories"`
	}) error {
		for _, entry := range in.FactoryEntries {
			logger.Infof("Register factory [%T] for id's [%v] and initiators [%v]", entry.factory, entry.fids, entry.initiators)
			for _, fid := range entry.fids {
				if err := in.ViewProvider.RegisterFactory(fid, entry.factory); err != nil {
					return errors2.Wrapf(err, "failed to register factory [%T]", entry.factory)
				}
			}
			for _, initiator := range entry.initiators {
				if err := in.ViewProvider.RegisterResponderFactory(entry.factory, initiator); err != nil {
					return errors2.Wrapf(err, "failed to register responder factory [%T]", entry.factory)
				}
			}
		}
		return nil
	})
}

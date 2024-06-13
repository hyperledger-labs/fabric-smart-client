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
	)
}

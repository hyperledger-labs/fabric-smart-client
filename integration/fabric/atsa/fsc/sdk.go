/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"context"
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
)

type SDK struct {
	*fabricsdk.SDK
}

func NewSDK(registry node.Registry) *SDK {
	return &SDK{SDK: fabricsdk.NewSDK(registry)}
}

func (p *SDK) Install(ctx context.Context) error {
	if err := p.SDK.Install(ctx); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
		digutils.Register[*core.FSNProvider](p.Container()),
	)
}

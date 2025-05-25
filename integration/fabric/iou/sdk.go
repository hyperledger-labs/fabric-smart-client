/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iou

import (
	"context"
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
)

type SDK struct {
	dig2.SDK
}

func NewSDK(registry node.Registry) *SDK {
	return &SDK{SDK: sdk.NewSDK(registry)}
}

func (p *SDK) Install(ctx context.Context) error {
	if err := p.SDK.Install(ctx); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
	)
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package iou provides the IOU (I-Owe-You) topology, SDK and test helpers used by the Fabric
// suites built on the IOU flow. Its views and states subpackages also back the Fabric-X IOU
// suites.
package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

type SDK struct {
	dig.SDK
}

func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: sdk.NewSDK(registry)}
}

func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
	)
}

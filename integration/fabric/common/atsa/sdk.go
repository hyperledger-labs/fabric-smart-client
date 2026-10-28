/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package atsa provides the asset-transfer-secured-agreement topology and SDK. Its views,
// states and client subpackages are shared by the Fabric and Fabric-X atsa suites; see
// README.md for a walkthrough of the flow.
package atsa

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

type SDK struct {
	*fabricsdk.SDK
}

func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: fabricsdk.NewSDK(registry)}
}

func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[state.VaultService](p.Container()),
		digutils.Register[*core.FSNProvider](p.Container()),
	)
}

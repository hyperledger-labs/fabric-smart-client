/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mixednets

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	fabricxsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// SDK is the Fabric-x SDK, which also installs the Fabric driver, so one node
// serves a Fabric and a Fabric-x network.
type SDK struct {
	dig.SDK
}

func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: fabricxsdk.NewSDK(registry)}
}

func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}
	return errors.Join(
		digutils.Register[*core.FSNProvider](p.Container()),
	)
}

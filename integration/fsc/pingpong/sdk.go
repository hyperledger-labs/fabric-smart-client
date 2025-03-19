/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"errors"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	"go.opentelemetry.io/otel/trace"
)

type SDK struct {
	*sdk.SDK
}

func NewSDK(registry node.Registry) *SDK {
	return &SDK{SDK: sdk.NewSDK(registry)}
}

func (p *SDK) Install() error {
	if err := p.SDK.Install(); err != nil {
		return err
	}

	return errors.Join(
		digutils.Register[trace.TracerProvider](p.Container()),
		digutils.Register[driver.EndpointService](p.Container()),
		digutils.Register[view3.IdentityProvider](p.Container()),
		digutils.Register[node.ViewManager](p.Container()), // Need to add it as a field in the node
		digutils.Register[id.SigService](p.Container()),
	)
}

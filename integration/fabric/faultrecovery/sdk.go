/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// SDK is the Fabric SDK with the default channel's committer set to process
// views.Namespace. A Fabric committer skips a transaction it has no record of unless
// one of its rules selects it, such as a namespace registered with ProcessNamespace,
// so without this a node that lost its vault would skip every faultrecovery
// transaction it replays. With it, the node commits every faultrecovery transaction
// whatever its vault holds, which would hide a lost Busy record, so only the specs
// that wipe a vault run on it.
type SDK struct {
	dig.SDK
}

func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: sdk.NewSDK(registry)}
}

// Start registers the namespace after the Fabric SDK has started and before its
// PostStart starts delivery, so the first replayed block already sees it.
func (p *SDK) Start(ctx context.Context) error {
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}
	return p.Container().Invoke(func(nsp *fabric.NetworkServiceProvider) error {
		fns, err := nsp.FabricNetworkService("")
		if err != nil {
			return errors.WithMessage(err, "failed getting default network")
		}
		ch, err := fns.Channel("")
		if err != nil {
			return errors.WithMessage(err, "failed getting default channel")
		}
		return ch.Committer().ProcessNamespace(views.Namespace)
	})
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package sdk holds the Fabric SDK of the nodes that process the faultrecovery namespace,
// views.Namespace.
package sdk

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// SDK is the Fabric SDK with the default channel's committer set to process
// views.Namespace. A Fabric committer skips a transaction it has no record of unless one
// of its rules selects it, such as a namespace registered with ProcessNamespace. With
// this SDK a node commits every transaction of views.Namespace from its block, whether or
// not it took part in it:
//   - an endorser of the namespace commits a transaction whose block it processes before a
//     late responder handles the request, so the transaction ends Valid, not Busy;
//   - a node that lost its vault commits the transactions it replays.
//
// It also commits a transaction whose Busy record a node lost, so a spec that checks
// such a node does not run on it.
type SDK struct {
	dig.SDK
}

// NewSDK returns an SDK on top of a fresh Fabric SDK for registry. nwo calls it from
// the generated main of each FSC node in the topology.
func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: fabricsdk.NewSDK(registry)}
}

// Start registers the namespace after the Fabric SDK has started and before its
// PostStart starts delivery, so the first block delivered already sees it.
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

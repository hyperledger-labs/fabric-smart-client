/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"encoding/json"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// InjectNetwork carries the raw Fabric network configuration to inject at runtime into an FSC
// node that was started with Topology.MinimalFSCFabricConfig enabled (i.e. its core.yaml only
// contains `fabric:\n  enabled: true`).
type InjectNetwork struct {
	// Raw is the full `fabric:`-rooted extension YAML, as produced by
	// network.Network.RenderFSCFabricExtension.
	Raw []byte
	// Network is the name of the Fabric network described by Raw.
	Network string
}

// InjectNetworkView adds a Fabric network to the local, already-running FSC node at runtime, and
// performs the additional wiring (default RWSet processor, per-channel RWSet handler provider,
// and committer/delivery start) that the Fabric SDK would otherwise perform at node startup for
// networks known at boot time.
type InjectNetworkView struct {
	InjectNetwork
}

func (v *InjectNetworkView) Call(viewCtx view.Context) (any, error) {
	provider, err := core.GetFabricNetworkServiceProvider(viewCtx)
	assert.NoError(err, "failed getting fabric network service provider")
	fsnProvider, ok := provider.(*core.FSNProvider)
	assert.True(ok, "expected *core.FSNProvider")

	assert.NoError(fsnProvider.AddNetwork(v.Raw), "failed adding network [%s]", v.Network)

	driverFNS, err := fsnProvider.FabricNetworkService(v.Network)
	assert.NoError(err, "failed materializing fabric network service [%s]", v.Network)

	fns, err := fabric.GetFabricNetworkService(viewCtx, v.Network)
	assert.NoError(err, "failed getting fabric network service [%s]", v.Network)

	// (a) install the default RWSet processor, as the Fabric SDK does at startup for networks
	// known at boot time (see platform/fabric/sdk/dig/sdk.go's registerProcessorsForDrivers).
	assert.NoError(
		fns.ProcessorManager().SetDefaultProcessor(state.NewRWSetProcessor(fns)),
		"failed setting default RWSet processor for network [%s]", v.Network,
	)

	// (b) and (c): for every channel of the newly added network, register the endorser
	// transaction RWSet handler provider and start the committer and delivery services, exactly
	// as the Fabric SDK does for networks configured at startup (see
	// platform/fabric/sdk/dig/sdk.go's registerRWSetLoaderHandlerProviders and
	// platform/fabric/core.FSNProvider.Start). The network was added after the node's lifecycle
	// context was created, so a detached context is used for the committer/delivery goroutines.
	ctx := context.Background()
	for _, channelName := range driverFNS.ConfigService().ChannelIDs() {
		ch, err := driverFNS.Channel(channelName)
		assert.NoError(err, "failed getting channel [%s] for network [%s]", channelName, v.Network)

		assert.NoError(
			ch.RWSetLoader().AddHandlerProvider(common.HeaderType_ENDORSER_TRANSACTION, rwset.NewEndorserTransactionHandler),
			"failed adding rwset handler provider for channel [%s]", channelName,
		)
		assert.NoError(ch.Committer().Start(ctx), "failed starting committer for channel [%s]", channelName)
		assert.NoError(ch.Delivery().Start(ctx), "failed starting delivery for channel [%s]", channelName)
	}

	return "OK", nil
}

type InjectNetworkViewFactory struct{}

func (f *InjectNetworkViewFactory) NewView(in []byte) (view.View, error) {
	v := &InjectNetworkView{}
	assert.NoError(json.Unmarshal(in, &v.InjectNetwork))
	return v, nil
}

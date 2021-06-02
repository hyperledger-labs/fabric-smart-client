/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

import (
	"reflect"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

func GetWorldStateService(ctx view2.ServiceProvider) WorldStateService {
	s, err := ctx.GetService(reflect.TypeOf((*WorldStateService)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(WorldStateService)
}

func GetWorldState(ctx view2.ServiceProvider) WorldState {
	ws, err := GetWorldStateService(ctx).GetWorldState(
		fabric.GetDefaultNetwork(ctx).Name(),
		fabric.GetDefaultChannel(ctx).Name(),
	)
	if err != nil {
		panic(err)
	}
	return ws
}

func GetWorldStateForChannel(ctx view2.ServiceProvider, channel string) WorldState {
	ws, err := GetWorldStateService(ctx).GetWorldState(
		fabric.GetDefaultNetwork(ctx).Name(),
		channel,
	)
	if err != nil {
		panic(err)
	}
	return ws
}

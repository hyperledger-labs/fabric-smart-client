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

func GetVaultService(ctx view2.ServiceProvider) VaultService {
	s, err := ctx.GetService(reflect.TypeOf((*VaultService)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(VaultService)
}

func GetVault(ctx view2.ServiceProvider) Vault {
	ws, err := GetVaultService(ctx).Vault(
		fabric.GetDefaultNetwork(ctx).Name(),
		fabric.GetDefaultChannel(ctx).Name(),
	)
	if err != nil {
		panic(err)
	}
	return ws
}

func GetVaultForChannel(ctx view2.ServiceProvider, channel string) Vault {
	ws, err := GetVaultService(ctx).Vault(
		fabric.GetDefaultNetwork(ctx).Name(),
		channel,
	)
	if err != nil {
		panic(err)
	}
	return ws
}

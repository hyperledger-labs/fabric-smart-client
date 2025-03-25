/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

func GetVaultService(ctx view2.ServiceProvider) (VaultService, error) {
	s, err := ctx.GetService(reflect.TypeOf((*VaultService)(nil)))
	if err != nil {
		return nil, err
	}
	return s.(VaultService), nil
}

func GetVault(ctx view2.ServiceProvider, opts ...ServiceOption) (Vault, error) {
	opt, err := CompileServiceOptions(opts...)
	if err != nil {
		return nil, err
	}
	vs, err := GetVaultService(ctx)
	if err != nil {
		return nil, err
	}
	if len(opt.Network) > 0 && len(opt.Channel) > 0 {
		return vs.Vault(opt.Network, opt.Channel)
	}
	fns, err := fabric.GetFabricNetworkService(ctx, opt.Network)
	if err != nil {
		return nil, err
	}
	ch, err := fns.Channel(opt.Channel)
	if err != nil {
		return nil, err
	}
	return vs.Vault(fns.Name(), ch.Name())
}

func GetVaultForChannel(ctx view2.ServiceProvider, channel string) (Vault, error) {
	return GetVault(ctx, WithChannel(channel))
}

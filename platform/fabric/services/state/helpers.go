/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"
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

func GetVault(ctx context.Context, sp view2.ServiceProvider) (Vault, error) {
	vs, err := GetVaultService(sp)
	if err != nil {
		return nil, err
	}
	fsc, ch, err := fabric.GetDefaultChannel(ctx, sp)
	if err != nil {
		return nil, err
	}
	ws, err := vs.Vault(fsc.Name(), ch.Name())
	if err != nil {
		return nil, err
	}
	return ws, nil
}

func GetVaultForChannel(ctx context.Context, sp view2.ServiceProvider, channel string) (Vault, error) {
	vs, err := GetVaultService(sp)
	if err != nil {
		return nil, err
	}
	fns, err := fabric.GetDefaultFNS(ctx, sp)
	if err != nil {
		return nil, err
	}
	ws, err := vs.Vault(fns.Name(), channel)
	if err != nil {
		return nil, err
	}
	return ws, nil
}

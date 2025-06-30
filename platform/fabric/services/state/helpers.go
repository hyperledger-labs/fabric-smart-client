/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

func GetVaultService(ctx services.Provider) (VaultService, error) {
	s, err := ctx.GetService(reflect.TypeOf((*VaultService)(nil)))
	if err != nil {
		return nil, err
	}
	return s.(VaultService), nil
}

func GetVault(ctx services.Provider) (Vault, error) {
	vs, err := GetVaultService(ctx)
	if err != nil {
		return nil, err
	}
	fsc, ch, err := fabric.GetDefaultChannel(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := vs.Vault(fsc.Name(), ch.Name())
	if err != nil {
		return nil, err
	}
	return ws, nil
}

func GetVaultForChannel(ctx services.Provider, channel string) (Vault, error) {
	vs, err := GetVaultService(ctx)
	if err != nil {
		return nil, err
	}
	fns, err := fabric.GetDefaultFNS(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := vs.Vault(fns.Name(), channel)
	if err != nil {
		return nil, err
	}
	return ws, nil
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	genericconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	fvault "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

// newStoreOpener opens the Fabric stores: the endorse tx, metadata and
// envelope stores, and the vault of every channel configured for a network
// that uses the generic driver. Other drivers, such as fabricx, keep no local
// vault. It opens every store even if one fails, and joins their errors with
// viewsdk.ErrMemoryFallback if one of them uses the memory driver by fallback.
func newStoreOpener(c commondig.Container) viewsdk.StoreOpenerEntry {
	return viewsdk.StoreOpenerEntry{Opener: viewsdk.StoreOpenerFunc(func(context.Context) error {
		errs := []error{
			c.Invoke(func(driver.EndorseTxStore) {}),
			c.Invoke(func(driver.MetadataStore) {}),
			c.Invoke(func(driver.EnvelopeStore) {}),
		}
		errs = append(errs, c.Invoke(func(networks *core.Config, configs config.Provider, drivers multiplexed.Driver) error {
			var vaultErrs []error
			for _, network := range networks.Names() {
				cs, err := configs.GetConfig(network)
				if err != nil {
					vaultErrs = append(vaultErrs, errors.WithMessagef(err, "failed reading configuration of network [%s]", network))
					continue
				}
				if cs.DriverName() != genericconfig.GenericDriver {
					continue
				}
				for _, channel := range cs.ChannelIDs() {
					if _, err := fvault.NewStore(cs.VaultPersistenceName(), drivers, network, channel); err != nil {
						vaultErrs = append(vaultErrs, errors.WithMessagef(err, "failed opening vault of channel [%s] of network [%s]", channel, network))
					}
				}
			}
			if drivers.FellBackToMemory() {
				vaultErrs = append(vaultErrs, viewsdk.ErrMemoryFallback)
			}
			return errors.Join(vaultErrs...)
		}))
		return errors.Join(errs...)
	})}
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
)

func NewStore(name driver.PersistenceName, d multiplexed.Driver, params ...string) (driver.VaultStore, error) {
	return d.NewVault(name, params...)
}

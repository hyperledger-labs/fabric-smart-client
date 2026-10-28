/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	fdbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func NewStore(name driver.PersistenceName, d multiplexed.Driver, params ...string) (fdbdriver.VaultStore, error) {
	return d.NewVault(name, params...)
}

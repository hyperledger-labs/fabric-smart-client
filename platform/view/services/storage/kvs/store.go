/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
)

func NewKeyValueStore(cp driver.Config, d multiplexed.Driver, params ...string) (driver.KeyValueStore, error) {
	return d.NewKVS(common.GetPersistenceName(cp, "fsc.kvs.persistence"), params...)
}

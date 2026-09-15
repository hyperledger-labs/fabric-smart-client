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

// NewKeyValueStore constructs a KeyValueStore backed by the persistence configured under
// "fsc.kvs.persistence", resolved through d.
func NewKeyValueStore(cp driver.Config, d multiplexed.Driver, params ...string) (driver.KeyValueStore, error) {
	name, err := common.GetPersistenceName(cp, "fsc.kvs.persistence")
	if err != nil {
		return nil, err
	}
	return d.NewKVS(name, params...)
}

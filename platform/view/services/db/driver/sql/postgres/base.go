/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type BasePersistence[V any, R any] struct {
	*common.BasePersistence[V, R]
}

func (db *BasePersistence[V, R]) SetStates(ns driver.Namespace, kvs map[driver.PKey]V) map[driver.PKey]error {
	return db.BasePersistence.SetStates(ns, kvs)
}

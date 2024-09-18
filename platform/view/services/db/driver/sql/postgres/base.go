/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/pkg/errors"
)

type BasePersistence[V any, R any] struct {
	*common.BasePersistence[V, R]

	table        string
	ci           common.Interpreter
	errorWrapper driver2.SQLErrorWrapper
}

func (db *BasePersistence[V, R]) SetStates(ns driver.Namespace, kvs map[driver.PKey]V) map[driver.PKey]error {
	return db.BasePersistence.SetStates(ns, kvs)
}

func (db *BasePersistence[V, R]) DeleteState(ns driver.Namespace, key driver.PKey) error {
	if errs := db.DeleteStates(ns, key); errs != nil {
		return errs[key]
	}
	return nil
}

func (db *BasePersistence[V, R]) DeleteStates(namespace driver.Namespace, keys ...driver.PKey) map[driver.PKey]error {
	if db.Txn == nil {
		panic("programming error, writing without ongoing update")
	}
	if namespace == "" {
		return collections.RepeatValue(keys, errors.New("ns or key is empty"))
	}
	where, args := common.Where(db.hasKeys(namespace, keys))
	query := fmt.Sprintf("DELETE FROM %s %s", db.table, where)
	logger.Debug(query, args)
	_, err := db.Txn.Exec(query, args...)
	if err != nil {
		errs := make(map[driver.PKey]error)
		for _, key := range keys {
			errs[key] = errors.Wrapf(db.errorWrapper.WrapError(err), "could not delete val for key [%s]", key)
		}
		return errs
	}

	return nil
}

func (db *BasePersistence[V, R]) hasKeys(ns driver.Namespace, pkeys []driver.PKey) common.Condition {
	return db.ci.And(
		db.ci.Cmp("ns", "=", ns),
		db.ci.InStrings("pkey", pkeys),
	)
}

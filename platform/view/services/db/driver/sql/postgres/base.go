/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type BasePersistence[V any, R any] struct {
	*common.BasePersistence[V, R]

	table        string
	ci           common.Interpreter
	errorWrapper driver2.SQLErrorWrapper
}

func (db *BasePersistence[V, R]) SetState(ns driver.Namespace, pkey driver.PKey, value V) error {
	return db.SetStateWithTx(db.Txn, ns, pkey, value)
}

func (db *BasePersistence[V, R]) SetStates(ns driver.Namespace, kvs map[driver.PKey]V) map[driver.PKey]error {
	return db.setStatesWithTx(db.Txn, ns, kvs)
}

func (db *BasePersistence[V, R]) SetStateWithTx(tx *sql.Tx, ns driver.Namespace, pkey driver.PKey, value V) error {
	if errs := db.setStatesWithTx(tx, ns, map[driver.PKey]V{pkey: value}); errs != nil {
		return errs[pkey]
	}
	return nil
}

func (db *BasePersistence[V, R]) setStatesWithTx(tx *sql.Tx, ns driver.Namespace, kvs map[driver.PKey]V) map[driver.PKey]error {
	if tx == nil {
		panic("programming error, writing without ongoing update")
	}
	keys := db.ValueScanner.Columns()
	valIndex := slices.Index(keys, "val")
	upserted := make(map[driver.PKey][]any, len(kvs))
	deleted := make([]driver.PKey, 0, len(kvs))
	for pkey, value := range kvs {
		values := db.ValueScanner.WriteValue(value)
		// Get rawVal
		if val := values[valIndex].([]byte); len(val) == 0 {
			logger.Debugf("set key [%s:%s] to nil value, will be deleted instead", ns, pkey)
			deleted = append(deleted, pkey)
		} else {
			logger.Debugf("set state [%s,%s]", ns, pkey)
			// Overwrite rawVal
			val = append([]byte(nil), val...)
			values[valIndex] = val
			upserted[pkey] = values
		}
	}

	errs := make(map[driver.PKey]error)
	if len(deleted) > 0 {
		collections.CopyMap(errs, db.DeleteStatesWithTx(tx, ns, deleted...))
	}
	if len(upserted) > 0 {
		collections.CopyMap(errs, db.upsertStatesWithTx(tx, ns, keys, upserted))
	}
	return errs
}

func (db *BasePersistence[V, R]) UpsertStates(ns driver.Namespace, valueKeys []string, vals map[driver.PKey][]any) map[driver.PKey]error {
	return db.upsertStatesWithTx(db.Txn, ns, valueKeys, vals)
}

func (db *BasePersistence[V, R]) upsertStatesWithTx(tx *sql.Tx, ns driver.Namespace, valueKeys []string, vals map[driver.PKey][]any) map[driver.PKey]error {
	keys := append([]string{"ns", "pkey"}, valueKeys...)
	query := fmt.Sprintf("INSERT INTO %s (%s) "+
		"VALUES %s "+
		"ON CONFLICT (ns, pkey) DO UPDATE "+
		"SET %s",
		db.table,
		strings.Join(keys, ", "),
		common.CreateParamsMatrix(len(keys), len(vals), 1),
		strings.Join(substitutions(valueKeys), ", "))

	args := make([]any, 0, len(keys)*len(vals))
	for pkey, vals := range vals {
		args = append(append(args, ns, pkey), vals...)
	}
	logger.Debug(query, args)
	if _, err := tx.Exec(query, args...); err != nil {
		return collections.RepeatValue(collections.Keys(vals), errors2.Wrapf(db.errorWrapper.WrapError(err), "could not upsert"))
	}
	return nil
}

// TODO: AF Needs to be calculated only once
func substitutions(keys []string) []string {
	subs := make([]string, len(keys))
	for i, key := range keys {
		subs[i] = fmt.Sprintf("%s = excluded.%s", key, key)
	}
	return subs
}

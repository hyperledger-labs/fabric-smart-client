/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"database/sql"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/pkg/errors"
)

type base struct {
	db         *sql.DB
	txn        *sql.Tx
	txnLock    sync.Mutex
	debugStack []byte
	table      string
}

func (db *base) Close() error {
	logger.Info("closing database")

	// TODO: what to do with db.txn if it's not nil?

	err := db.db.Close()
	if err != nil {
		return errors.Wrap(err, "could not close DB")
	}

	return nil
}

func (db *base) BeginUpdate() error {
	logger.Debug(fmt.Sprintf("begin db transaction on table %s", db.table))
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn != nil {
		logger.Errorf("previous commit in progress, locked by [%s]", db.debugStack)
		return errors.New("previous commit in progress")
	}

	tx, err := db.db.Begin()
	if err != nil {
		return errors.Wrap(err, "error starting db transaction")
	}
	db.txn = tx
	db.debugStack = debug.Stack()

	return nil
}

func (db *base) Commit() error {
	logger.Debug(fmt.Sprintf("commit db transaction on table %s", db.table))
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	err := db.txn.Commit()
	if err != nil {
		return errors.Wrap(err, "could not commit transaction")
	}
	db.txn = nil

	return nil
}

func (db *base) Discard() error {
	logger.Debug(fmt.Sprintf("rollback db transaction on table %s", db.table))
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}
	err := db.txn.Rollback()
	if err != nil {
		logger.Infof("error rolling back (ignoring): %s", err.Error())
		return nil
	}
	db.txn = nil

	return nil
}

func (db *base) exists(tx *sql.Tx, ns, key string) (bool, error) {
	var pkey string
	query := fmt.Sprintf("SELECT pkey FROM %s WHERE ns = $1 AND pkey = $2", db.table)
	logger.Debug(query, ns, key)
	err := tx.QueryRow(query, ns, key).Scan(&pkey)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "cannot check if key exists")
	}
	return true, nil
}

func (db *base) DeleteState(ns, key string) error {
	if db.txn == nil {
		panic("programming error, writing without ongoing update")
	}
	if ns == "" || key == "" {
		return errors.New("ns or key is empty")
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE ns = $1 AND pkey = $2", db.table)
	logger.Debug(query, ns, key)
	_, err := db.txn.Exec(query, ns, key)
	if err != nil {
		return errors.Wrapf(err, "could not delete val for key %s", key)
	}

	return nil
}

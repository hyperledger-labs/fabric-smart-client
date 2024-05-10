/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("db.driver.sql")

type base struct {
	writeDB    *sql.DB
	readDB     *sql.DB
	txn        *sql.Tx
	txnLock    sync.Mutex
	debugStack []byte
	table      string
}

func (db *base) Close() error {
	logger.Info("closing database")

	// TODO: what to do with db.txn if it's not nil?

	err := db.writeDB.Close()
	if err != nil {
		return fmt.Errorf("could not close DB: %w", err)
	}

	return nil
}

func (db *base) BeginUpdate() error {
	logger.Debugf("begin db transaction [%s]", db.table)
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn != nil {
		logger.Errorf("previous commit in progress, locked by [%s]", db.debugStack)
		return errors.New("previous commit in progress")
	}

	tx, err := db.writeDB.Begin()
	if err != nil {
		return fmt.Errorf("error starting db transaction: %w", err)
	}
	db.txn = tx
	db.debugStack = debug.Stack()

	return nil
}

func (db *base) Commit() error {
	logger.Debugf("commit db transaction [%s]", db.table)
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.txn == nil {
		return errors.New("no commit in progress")
	}

	err := db.txn.Commit()
	db.txn = nil
	if err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}

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
	db.txn = nil
	if err != nil {
		logger.Infof("error rolling back (ignoring): %s", err.Error())
		return nil
	}

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
		return false, fmt.Errorf("cannot check if key exists: %w", err)
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
		return fmt.Errorf("could not delete val for key [%s]: %w", key, err)
	}

	return nil
}

type Opts struct {
	Driver          string
	DataSource      string
	TablePrefix     string
	SkipCreateTable bool
	SkipPragmas     bool
	MaxOpenConns    int
}

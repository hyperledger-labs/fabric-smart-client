/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"runtime/debug"
	"sync"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger("view-sdk.db.driver.common")

type DBTransaction interface {
	Commit() error
	Rollback() error
}

type BaseDB[T DBTransaction] struct {
	Txn            T
	newTransaction func() (T, error)
	txnLock        sync.Mutex
	txLock         sync.Mutex
	debugStack     []byte
	zero           T
}

func NewBaseDB[T DBTransaction](newTransaction func() (T, error)) *BaseDB[T] {
	return &BaseDB[T]{newTransaction: newTransaction}
}

func (db *BaseDB[T]) BeginUpdate() error {
	logger.Debugf("begin db transaction")
	db.txLock.Lock()
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if !db.IsTxnNil() {
		db.txLock.Unlock()
		logger.Errorf("previous commit in progress, locked by [%s]", db.debugStack)
		return errors.New("previous commit in progress")
	}

	tx, err := db.newTransaction()
	if err != nil {
		db.txLock.Unlock()
		return errors2.Wrapf(err, "error starting db transaction")
	}
	db.Txn = tx
	db.debugStack = debug.Stack()

	return nil
}

func (db *BaseDB[T]) Commit() error {
	logger.Debugf("commit db transaction")
	defer db.txLock.Unlock()
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.IsTxnNil() {
		return errors.New("no commit in progress")
	}

	err := db.Txn.Commit()
	db.Txn = db.zero
	db.debugStack = nil
	if err != nil {
		return errors2.Wrapf(err, "could not commit transaction")
	}

	return nil
}

func (db *BaseDB[T]) IsTxnNil() bool {
	return db.debugStack == nil
}

func (db *BaseDB[T]) Discard() error {
	logger.Debug("rollback db transaction")
	db.txnLock.Lock()
	defer db.txnLock.Unlock()

	if db.IsTxnNil() {
		return errors.New("no commit in progress")
	}
	defer db.txLock.Unlock()
	err := db.Txn.Rollback()
	db.Txn = db.zero
	db.debugStack = nil
	if err != nil {
		logger.Debugf("error rolling back (ignoring): %s", err.Error())
		return nil
	}

	return nil
}

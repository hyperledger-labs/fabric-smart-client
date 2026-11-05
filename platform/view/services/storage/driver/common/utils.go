/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// RWDB holds the connection pools of a persistence. Persistences that share a
// data source share the pools.
type RWDB struct {
	ReadDB  *sql.DB
	WriteDB *sql.DB
	// QueryTimeout bounds each statement of the stores built from this RWDB; zero
	// adds no deadline. It belongs to one persistence, so a DbProvider leaves it
	// zero and NewPersistenceWithOpts sets it on a copy.
	QueryTimeout time.Duration
}

type DBObject interface {
	CreateSchema() error
}

func CopyPtr[T any](t T) *T {
	v := t
	return &v
}

func Close(readDB, writeDB *sql.DB) error {
	if readDB == writeDB {
		return readDB.Close()
	}
	return errors.Join(readDB.Close(), writeDB.Close())
}

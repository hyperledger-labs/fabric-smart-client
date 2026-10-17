/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type RWDB struct {
	ReadDB  *sql.DB
	WriteDB *sql.DB
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

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import "database/sql"

type RWDB struct {
	ReadDB  *sql.DB
	WriteDB *sql.DB
}

type DBObject interface {
	CreateSchema() error
}
type PersistenceConstructor[O any, V DBObject] func(O) (V, error)

func CopyPtr[T any](t T) *T {
	v := t
	return &v
}

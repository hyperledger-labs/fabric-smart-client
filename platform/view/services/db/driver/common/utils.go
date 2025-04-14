/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

type DBObject interface {
	CreateSchema() error
}
type PersistenceConstructor[O any, V DBObject] func(O, string) (V, error)

func CopyPtr[T any](t T) *T {
	v := t
	return &v
}

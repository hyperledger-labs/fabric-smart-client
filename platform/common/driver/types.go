/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type NamedDriver[D any] struct {
	Name   PersistenceType
	Driver D
}

type (
	TxID      = string
	BlockNum  = uint64
	TxNum     = uint64
	Namespace = string
)

type PersistenceType string

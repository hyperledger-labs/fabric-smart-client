/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type (
	GetStateOpt = driver.GetStateOpt
	RWSet       = driver.RWSet
)

type RWSetLoader interface {
	GetRWSetFromEvn(txID string) (RWSet, ProcessTransaction, error)
	GetRWSetFromETx(txID string) (RWSet, ProcessTransaction, error)
	GetInspectingRWSetFromEvn(id string, envelopeRaw []byte) (RWSet, ProcessTransaction, error)
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type (
	Namespace  = driver.Namespace
	PKey       = driver.PKey
	VaultValue = driver.VaultValue
)

type QueryService interface {
	GetState(ns Namespace, key PKey) (*VaultValue, error)
	GetStates(map[Namespace][]PKey) (map[Namespace]map[PKey]VaultValue, error)
	GetTransactionStatus(txID string) (int32, error)
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/driver"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
)

type (
	ValidationCode = driver.ValidationCode
	Namespace      = driver.Namespace
	PKey           = driver.PKey
	VaultValue     = driver.VaultValue
)

const (
	Valid   = driver.Valid
	Invalid = driver.Invalid
	Unknown = driver.Unknown
	Busy    = driver.Busy
)

type QueryService struct {
	driver.QueryService
}

func NewQueryService(queryService driver.QueryService) *QueryService {
	return &QueryService{QueryService: queryService}
}

func (q *QueryService) GetTxStatus(txID string) (ValidationCode, error) {
	c, err := q.GetTransactionStatus(txID)
	if err != nil {
		return Unknown, err
	}
	switch protoblocktx.Status(c) {
	case protoblocktx.Status_NOT_VALIDATED:
		return Unknown, nil
	case protoblocktx.Status_COMMITTED:
		return Valid, nil
	default:
		return Invalid, nil
	}
}

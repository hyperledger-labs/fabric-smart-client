/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/driver"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
)

type (
	ValidationCode = fdriver.ValidationCode
	Namespace      = driver.Namespace
	PKey           = driver.PKey
	VaultValue     = driver.VaultValue
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
		return fdriver.Unknown, err
	}
	switch protoblocktx.Status(c) {
	case protoblocktx.Status_NOT_VALIDATED:
		return fdriver.Unknown, nil
	case protoblocktx.Status_COMMITTED:
		return fdriver.Valid, nil
	default:
		return fdriver.Invalid, nil
	}
}

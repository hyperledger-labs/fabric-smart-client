/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/driver"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
)

type ValidationCode = driver2.ValidationCode

type QueryService struct {
	driver.QueryService
}

func NewQueryService(queryService driver.QueryService) *QueryService {
	return &QueryService{QueryService: queryService}
}

func (q *QueryService) GetTxStatus(txID string) (ValidationCode, error) {
	c, err := q.GetTransactionStatus(txID)
	if err != nil {
		return driver2.Unknown, err
	}
	switch protoblocktx.Status(c) {
	case protoblocktx.Status_NOT_VALIDATED:
		return driver2.Unknown, nil
	case protoblocktx.Status_COMMITTED:
		return driver2.Valid, nil
	default:
		return driver2.Invalid, nil
	}
}

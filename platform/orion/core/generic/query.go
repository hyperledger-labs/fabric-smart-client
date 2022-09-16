/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/pkg/errors"
)

type Query struct {
	query bcdb.Query
}

func (q *Query) GetDataByRange(dbName, startKey, endKey string, limit uint64) (driver.QueryIterator, error) {
	it, err := q.query.GetDataByRange(dbName, startKey, endKey, limit)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting data by range")
	}
	return it, nil
}

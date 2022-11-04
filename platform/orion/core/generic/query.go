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
	return &QueryIterator{it: it}, nil
}

type QueryIterator struct {
	it bcdb.Iterator
}

func (q *QueryIterator) Next() (string, []byte, uint64, uint64, bool, error) {
	v, b, err := q.it.Next()
	if err != nil {
		return "", nil, 0, 0, false, err
	}

	if !b {
		return "", nil, 0, 0, false, nil
	}

	return v.Key, v.Value, v.Metadata.Version.BlockNum, v.Metadata.Version.TxNum, true, nil
}

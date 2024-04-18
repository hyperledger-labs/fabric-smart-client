/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"

type AggregatedTransactionFilter struct {
	filters []driver.TransactionFilter
}

func NewAggregatedTransactionFilter() *AggregatedTransactionFilter {
	return &AggregatedTransactionFilter{filters: []driver.TransactionFilter{}}
}

func (f *AggregatedTransactionFilter) Add(filter driver.TransactionFilter) {
	f.filters = append(f.filters, filter)
}

func (f *AggregatedTransactionFilter) Accept(txID string, env []byte) (bool, error) {
	for _, filter := range f.filters {
		ok, err := filter.Accept(txID, env)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

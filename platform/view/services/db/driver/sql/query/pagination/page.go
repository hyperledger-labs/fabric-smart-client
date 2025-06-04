/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

// NewPage creates a new page where the id is a string
func NewPage[V any](results collections.Iterator[*V], pagination driver.Pagination) (*driver.PageIterator[*V], error) {
	return NewTypedPage[string, V](results, pagination)
}

// NewTypedPage creates a new page from the results and the previous pagination
func NewTypedPage[I comparable, V any](results iterators.Iterator[*V], pagination driver.Pagination) (*driver.PageIterator[*V], error) {
	if p, ok := pagination.(*keyset[I, *V]); ok {
		items, err := iterators.ReadAllPointers(results)
		if err != nil {
			return nil, err
		}
		p.lastId = p.idGetter(items[len(items)-1])
		return &driver.PageIterator[*V]{Items: collections.NewSliceIterator[*V](items), Pagination: p}, nil
	}
	return &driver.PageIterator[*V]{Items: results, Pagination: pagination}, nil
}

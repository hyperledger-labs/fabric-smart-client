/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

// NewPage creates a new page where the id is a string
func NewPage[V any](results collections.Iterator[*V], pagination driver.Pagination) (*driver.PageIterator[*V], error) {
	// Check if pagination is a keyset[int, interface{}]
	if _, ok := pagination.(*keyset[int, interface{}]); ok {
		return NewTypedPage[int, V](results, pagination)
	}

	// Check if pagination is a keyset[string, interface{}]
	if _, ok := pagination.(*keyset[string, interface{}]); ok {
		return NewTypedPage[string, V](results, pagination)
	}

	// If pagination is any other keyset type, panic
	if _, ok := pagination.(*keyset[any, any]); ok {
		panic("Unsupported keyset type in pagination")
	}

	// If pagination is not a keyset type, return NewTypedPage[string, V]
	return NewTypedPage[string, V](results, pagination)
}

// NewTypedPage creates a new page from the results and the previous pagination
func NewTypedPage[I comparable, V any](results iterators.Iterator[*V], pagination driver.Pagination) (*driver.PageIterator[*V], error) {
	fmt.Printf("type of pagination = %T\n", pagination)
	var v V
	fmt.Printf("type of V = %T\n", v)
	if p, ok := pagination.(*keyset[I, interface{}]); ok {
		items, err := iterators.ReadAllPointers(results)
		if err != nil {
			return nil, err
		}
		p.offsetOfLastId = p.offset + len(items)
		if len(items) == 0 {
			p.lastId = p.firstId
			return &driver.PageIterator[*V]{Items: collections.NewSliceIterator[*V](items), Pagination: p}, nil
		}
		item := items[len(items)-1]
		pv := p.idGetter(*item)
		p.lastId = pv
		return &driver.PageIterator[*V]{Items: collections.NewSliceIterator[*V](items), Pagination: p}, nil
	}
	return &driver.PageIterator[*V]{Items: results, Pagination: pagination}, nil
}

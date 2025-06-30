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
	switch p := pagination.(type) {
	case *keyset[int, interface{}]:
		return newKeysetTypedPage[int, V](results, p)
	case *keyset[string, interface{}]:
		return newKeysetTypedPage[string, V](results, p)
	case *offset:
		return &driver.PageIterator[*V]{Items: results, Pagination: pagination}, nil
	case *empty:
		return &driver.PageIterator[*V]{Items: results, Pagination: pagination}, nil
	case *none:
		return &driver.PageIterator[*V]{Items: results, Pagination: pagination}, nil
	default:
		panic("Unsupported pagination type")
	}
}

// NewTypedPage creates a new page from the results and the previous pagination
func newKeysetTypedPage[I comparable, V any](results iterators.Iterator[*V], pagination driver.Pagination) (*driver.PageIterator[*V], error) {
	p, ok := pagination.(*keyset[I, interface{}])
	if !ok {
		return nil, nil
	}
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

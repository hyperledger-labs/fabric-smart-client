/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

// Pagination describe a moving page
type Pagination interface {
	// Prev moves the pagination pointer to the prev page
	Prev() (Pagination, error)
	// Next moves the pagination pointer to the next page
	Next() (Pagination, error)
	// Equal checks whether a given pagination struct is of the same type and with the same parameters
	Equal(Pagination) bool
	// Serialize the pagination struct into a buffer
	Serialize() ([]byte, error)
}

// PageIterator is an ieterator with support for pagination
type PageIterator[R comparable] struct {
	Items      iterators.Iterator[R]
	Pagination Pagination
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

type Pagination interface {
	// Move the pagination pointer to the prev page
	Prev() (Pagination, error)
	// Move the pagination pointer to the next page
	Next() (Pagination, error)
	// Check whether a given pagination struct is of the same type and with the same parameters
	Equal(Pagination) bool
	// Serialize the pagination struct into a buffer
	Serialize() ([]byte, error)
}

type PageIterator[R comparable] struct {
	Items      iterators.Iterator[R]
	Pagination Pagination
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

type Pagination interface {
	Prev() (Pagination, error)
	Next() (Pagination, error)
}

type PageIterator[R comparable] struct {
	Items      iterators.Iterator[R]
	Pagination Pagination
}

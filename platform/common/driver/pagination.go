/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"

type Pagination interface {
	Prev() (Pagination, error)
	Next() (Pagination, error)
	First() (Pagination, error)
}

type PageIterator[R comparable] struct {
	Items      collections.Iterator[R]
	Pagination Pagination
}

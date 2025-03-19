/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"

type Pagination interface {
	Prev() Pagination
	Next() Pagination
	Last() Pagination
	First() Pagination
}

type PaginatedResponse[R comparable] struct {
	Items      collections.Iterator[R]
	Pagination Pagination
}

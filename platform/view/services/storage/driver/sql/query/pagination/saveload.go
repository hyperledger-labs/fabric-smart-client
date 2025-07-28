/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pagination

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

func SavePagination(p driver.Pagination) ([]byte, error) {
	switch pagination := p.(type) {
	case *none:
	case *offset:
	case *empty:
		panic("Save is supported only for keyset pagination")

	case *keyset[int, any]:
	case *keyset[string, any]:
		return pagination.SaveKeysetPagination()

	default:
		panic("Save is supported only for keyset pagination")
	}
	return nil, nil
}

func LoadPagination[I comparable, V any](data []byte) (keyset[I, V], error) {
	return LoadKeysetPagination[I, V](data)
}

func EqualKeysetPagination[I comparable](a, b *keyset[I, any]) bool {
	return a.Offset == b.Offset &&
		a.PageSize == b.PageSize &&
		a.SqlIdName == b.SqlIdName &&
		a.FirstId == b.FirstId &&
		a.LastId == b.LastId
	// Note: idGetter is not comparable and is intentionally skipped
}

func EqualPagination(a, b driver.Pagination) bool {
	switch ka := a.(type) {
	case *none:
	case *offset:
	case *empty:
		panic("Equal is supported only for keyset pagination")

	case *keyset[string, any]:
		kb, ok := b.(*keyset[string, any])
		if !ok {
			return false
		}
		return EqualKeysetPagination[string](ka, kb)

	case *keyset[int, any]:
		kb, ok := b.(*keyset[int, any])
		if !ok {
			return false
		}
		return EqualKeysetPagination[int](ka, kb)

	default:
		panic("Save is supported only for keyset pagination")
	}
	return false
}

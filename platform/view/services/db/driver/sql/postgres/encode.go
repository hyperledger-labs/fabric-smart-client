/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"encoding/hex"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

func identity(a string) (string, error) { return a, nil }

func decodeUnversionedReadIterator(it iterators.Iterator[*driver2.UnversionedRead], err error) (iterators.Iterator[*driver2.UnversionedRead], error) {
	return decodeIterator(it, err, decodeUnversionedRead)
}

func decodeIterator[R any](it iterators.Iterator[*R], err error, transformer func(v *R) (*R, error)) (iterators.Iterator[*R], error) {
	if err != nil {
		return nil, err
	}
	return collections.Map(it, transformer), nil
}

func decode(s string) (string, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), err
}

func decodeUnversionedRead(v *driver2.UnversionedRead) (*driver2.UnversionedRead, error) {
	if v == nil {
		return nil, nil
	}
	key, err := decode(v.Key)
	if err != nil {
		return nil, err
	}
	return &driver2.UnversionedRead{
		Key: key,
		Raw: v.Raw,
	}, nil
}

func encode(s string) string {
	return hex.EncodeToString([]byte(s))
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import (
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

func NewIterator[T any](fs ...func() (T, error)) *Iterator[T] {
	return &Iterator[T]{fs: fs}
}

type Iterator[T any] struct {
	fs []func() (T, error)
}

func (it *Iterator[T]) Next() (T, error) {
	if len(it.fs) == 0 {
		return utils.Zero[T](), io.EOF
	}
	result, err := it.fs[0]()
	it.fs = it.fs[1:]
	return result, err
}

func (it *Iterator[T]) Close() {
	it.fs = nil
}

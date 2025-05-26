/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

// Map lazily maps an iterator to another
func Map[A any, B any](iterator Iterator[A], transformer Transformer[A, B]) Iterator[B] {
	return &mapped[A, B]{Iterator: iterator, transformer: transformer}
}

type mapped[A any, B any] struct {
	Iterator[A]
	transformer func(A) (B, error)
}

func (it *mapped[A, B]) Next() (B, error) {
	if next, err := it.Iterator.Next(); err != nil {
		return utils.Zero[B](), err
	} else {
		return it.transformer(next)
	}
}

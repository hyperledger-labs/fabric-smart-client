/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

func FlattenValues[A any, B any](iterator Iterator[A], transformer Transformer[A, []B]) Iterator[*B] {
	return &flattenedValues[A, B]{Iterator: iterator, transformer: transformer, remaining: []B{}}
}

type flattenedValues[A any, B any] struct {
	Iterator[A]
	transformer func(A) ([]B, error)
	remaining   []B
}

func (it *flattenedValues[A, B]) Next() (*B, error) {
	if len(it.remaining) > 0 {
		n := it.remaining[0]
		it.remaining = it.remaining[1:]
		return &n, nil
	} else if next, err := it.Iterator.Next(); err != nil {
		return nil, errors.Wrapf(err, "failed fetching")
	} else if utils.IsNil(next) {
		return nil, nil
	} else if next, err := it.transformer(next); err != nil {
		return nil, errors.Wrapf(err, "failed transforming")
	} else if len(next) == 0 {
		return nil, nil
	} else {
		it.remaining = next[1:]
		return &next[0], nil
	}
}

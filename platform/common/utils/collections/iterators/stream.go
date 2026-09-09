/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import (
	"errors"
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

type stream[T any] interface {
	Recv() (T, error)
	CloseSend() error
}

func Stream[T any](cli stream[T]) Iterator[T] {
	return &streamIterator[T]{cli: cli}
}

type streamIterator[T any] struct {
	cli stream[T]
}

func (it *streamIterator[T]) Next() (T, error) {
	n, err := it.cli.Recv()
	if err == nil {
		return n, nil
	}
	if errors.Is(err, io.EOF) {
		return utils.Zero[T](), nil
	}
	return utils.Zero[T](), err
}

func (it *streamIterator[T]) Close() {
	_ = it.cli.CloseSend()
}

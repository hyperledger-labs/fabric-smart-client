/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators

import (
	"errors"
	"io"
)

type stream[T any] interface {
	Recv() (T, error)
	CloseSend() error
}

// Stream adapts a gRPC-style receive stream to an [Iterator]. Next turns
// io.EOF into the exhaustion signal (zero value, nil error); any other error
// from Recv is returned as-is. Close sends the stream's half-close.
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
	var zero T
	if errors.Is(err, io.EOF) {
		return zero, nil
	}
	return zero, err
}

func (it *streamIterator[T]) Close() {
	_ = it.cli.CloseSend()
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

// --- Stream ---------------------------------------------------------------

func TestStream(t *testing.T) {
	t.Parallel()

	s := &recvStream[int]{items: []int{1, 2, 3}, err: io.EOF}
	it := iterators.Stream[int](s)

	got := make([]int, 0)
	for {
		v, err := it.Next()
		require.NoError(t, err)
		if v == 0 {
			break
		}
		got = append(got, v)
	}
	require.Equal(t, []int{1, 2, 3}, got)
}

func TestStreamEOFEndsIteration(t *testing.T) {
	t.Parallel()

	it := iterators.Stream[int](&recvStream[int]{items: []int{}, err: io.EOF})

	v, err := it.Next()
	require.NoError(t, err, "io.EOF is exhaustion, not an error")
	require.Zero(t, v)
}

func TestStreamErrorPropagates(t *testing.T) {
	t.Parallel()

	recvErr := errors.New("stream broke")
	it := iterators.Stream[int](&recvStream[int]{items: []int{}, err: recvErr})

	_, err := it.Next()
	require.ErrorIs(t, err, recvErr)
}

// TestStreamErrorAfterItems checks a mid-stream failure surfaces after the
// successfully received elements.
func TestStreamErrorAfterItems(t *testing.T) {
	t.Parallel()

	recvErr := errors.New("stream broke late")
	it := iterators.Stream[int](&recvStream[int]{items: []int{7}, err: recvErr})

	first, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, 7, first)

	_, err = it.Next()
	require.ErrorIs(t, err, recvErr)
}

func TestStreamCloseCallsCloseSend(t *testing.T) {
	t.Parallel()

	s := &recvStream[int]{items: []int{}, err: io.EOF}
	iterators.Stream[int](s).Close()
	require.Equal(t, 1, s.closeCalls)
}

// TestStreamCloseIgnoresCloseSendError documents that Close swallows the error
// from CloseSend, since Iterator.Close returns nothing.
func TestStreamCloseIgnoresCloseSendError(t *testing.T) {
	t.Parallel()

	s := &recvStream[int]{items: []int{}, err: io.EOF, closeErr: errors.New("close failed")}
	require.NotPanics(t, func() { iterators.Stream[int](s).Close() })
	require.Equal(t, 1, s.closeCalls)
}

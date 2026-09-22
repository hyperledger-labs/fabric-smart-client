/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

func TestSerialRunner_Success(t *testing.T) {
	t.Parallel()

	callCount := 0
	runner := NewSerialRunner(func(vals []int) []error {
		callCount++
		require.Len(t, vals, 1, "should receive single item")
		return []error{nil}
	})

	err := runner.Run(42)
	require.NoError(t, err)
	require.Equal(t, 1, callCount)
}

func TestSerialRunner_Error(t *testing.T) {
	t.Parallel()

	runner := NewSerialRunner(func(vals []int) []error {
		require.Len(t, vals, 1)
		if vals[0] == 99 {
			return []error{errors.New("test error")}
		}
		return []error{nil}
	})

	err := runner.Run(99)
	require.Error(t, err)
	require.Contains(t, err.Error(), "test error")
}

func TestSerialExecutor_Success(t *testing.T) {
	t.Parallel()

	executor := NewSerialExecutor(func(inputs []string) []Output[int] {
		require.Len(t, inputs, 1)
		return []Output[int]{{Val: len(inputs[0]), Err: nil}}
	})

	val, err := executor.Execute("hello")
	require.NoError(t, err)
	require.Equal(t, 5, val)
}

func TestSerialExecutor_Error(t *testing.T) {
	t.Parallel()

	executor := NewSerialExecutor(func(inputs []string) []Output[int] {
		require.Len(t, inputs, 1)
		if inputs[0] == "error" {
			return []Output[int]{{Val: 0, Err: errors.New("test error")}}
		}
		return []Output[int]{{Val: len(inputs[0]), Err: nil}}
	})

	val, err := executor.Execute("error")
	require.Error(t, err)
	require.Equal(t, 0, val)
}

// An ExecuteFunc must return exactly one output per input. The serial
// implementations pair outputs to inputs positionally, so a short or empty
// result would index out of range; they report the mismatch instead.

func TestSerialRunner_EmptyResult(t *testing.T) {
	t.Parallel()

	runner := NewSerialRunner(func([]int) []error {
		return nil
	})

	err := runner.Run(42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 output, but got 0")
}

func TestSerialRunner_TooManyResults(t *testing.T) {
	t.Parallel()

	runner := NewSerialRunner(func([]int) []error {
		return []error{nil, nil}
	})

	err := runner.Run(42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 output, but got 2")
}

func TestSerialExecutor_EmptyResult(t *testing.T) {
	t.Parallel()

	executor := NewSerialExecutor(func([]string) []Output[int] {
		return nil
	})

	val, err := executor.Execute("hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 output, but got 0")
	require.Zero(t, val)
}

func TestSerialExecutor_TooManyResults(t *testing.T) {
	t.Parallel()

	executor := NewSerialExecutor(func([]string) []Output[int] {
		return []Output[int]{{Val: 1}, {Val: 2}}
	})

	val, err := executor.Execute("hello")
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1 output, but got 2")
	require.Zero(t, val)
}

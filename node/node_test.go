/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type exitPanic struct {
	code int
}

func TestNewWithConfPathInvalidConfigReturnsStartError(t *testing.T) {
	n := NewWithConfPath("./does-not-exist")
	require.Error(t, n.Start())
}

func TestListenExitsOnStartupError(t *testing.T) {
	originalExit := exit
	defer func() { exit = originalExit }()

	n := &Node{callbackChannel: make(chan error, 1)}
	exit = func(code int) { panic(exitPanic{code: code}) }

	done := make(chan interface{}, 1)
	go func() {
		defer func() {
			done <- recover()
		}()
		n.listen()
	}()

	n.callbackChannel <- errors.New("boom")
	recovered := <-done
	require.Equal(t, exitPanic{code: 1}, recovered)
}

func TestListenExitsOnCallbackError(t *testing.T) {
	originalExit := exit
	defer func() { exit = originalExit }()

	n := &Node{
		callbackChannel: make(chan error, 1),
		executeCallbackFunc: func() error {
			return errors.New("callback failed")
		},
	}
	exit = func(code int) { panic(exitPanic{code: code}) }

	done := make(chan interface{}, 1)
	go func() {
		defer func() {
			done <- recover()
		}()
		n.listen()
	}()

	n.callbackChannel <- nil
	recovered := <-done
	require.Equal(t, exitPanic{code: 1}, recovered)
}

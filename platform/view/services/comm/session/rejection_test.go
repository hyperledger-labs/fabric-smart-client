/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package session

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestRejectionError(t *testing.T) {
	t.Parallel()
	sentinels := []error{ErrRejected, ErrUnknownView, ErrBusy, ErrDraining, ErrUnauthorized, view.ErrNoCommonVersion}
	for payload, want := range map[string]error{
		"unknown_view: app/View":       ErrUnknownView,
		"busy: too many":               ErrBusy,
		"node_busy: too many":          ErrBusy,
		"draining: node is draining":   ErrDraining,
		"unauthorized: refused":        ErrUnauthorized,
		"authorizer_error: timed out":  ErrRejected,
		"no_common_version: view 2":    view.ErrNoCommonVersion,
		"busy":                         nil, // no ": " after the reason
		"boom: unknown_view: app/View": nil, // a reason not at the start
		"responder failed":             nil,
		"":                             nil,
	} {
		t.Run(payload, func(t *testing.T) {
			t.Parallel()
			err := RejectionError(&view.Message{Status: view.ERROR, Payload: []byte(payload)})
			require.EqualError(t, err, "received error from remote ["+payload+"]")
			for _, s := range sentinels {
				expected := want != nil && (errors.Is(want, s) || errors.Is(s, ErrRejected))
				require.Equal(t, expected, errors.Is(err, s), "errors.Is(%v)", s)
			}
		})
	}
}

// TestReceiveHelpersReturnRejectionError checks that every receive helper types a rejection.
func TestReceiveHelpersReturnRejectionError(t *testing.T) {
	t.Parallel()
	rejection := func() *mockSession {
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.ERROR, Payload: []byte("unauthorized: refused")}
		return &mockSession{ch: ch}
	}
	check := func(t *testing.T, err error) {
		t.Helper()
		require.ErrorIs(t, err, ErrUnauthorized)
		require.EqualError(t, err, "received error from remote [unauthorized: refused]")
	}

	t.Run("ReadMessageWithTimeout", func(t *testing.T) {
		t.Parallel()
		_, err := ReadMessageWithTimeout(rejection(), time.Second)
		check(t, err)
	})
	t.Run("ReadFirstMessage", func(t *testing.T) {
		t.Parallel()
		_, _, err := ReadFirstMessage(&mockContext{s: rejection()})
		check(t, err)
	})
	t.Run("ReadFirstMessageOrPanic", func(t *testing.T) {
		t.Parallel()
		defer func() {
			err, ok := recover().(error)
			require.True(t, ok)
			check(t, err)
		}()
		ReadFirstMessageOrPanic(&mockContext{s: rejection()})
	})
	t.Run("json ReceiveRaw", func(t *testing.T) {
		t.Parallel()
		_, err := newJSONSession(rejection(), t.Context()).ReceiveRaw()
		check(t, err)
	})
}

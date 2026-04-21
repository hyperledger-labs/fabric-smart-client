/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type networkNode interface {
	NewResponderSession(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error)
	Start(ctx context.Context)
	ID() string
}

func SessionTwoParties(t *testing.T, network ...networkNode) {
	t.Helper()
	ctx := t.Context()
	for _, node := range network {
		node.Start(ctx)
	}

	session01, err := network[0].NewResponderSession(
		"session_id", "context_id", "", []byte(network[1].ID()), nil, nil)
	require.NoError(t, err)
	conn01, err := NewConn(0, session01)
	require.NoError(t, err)

	session10, err := network[1].NewResponderSession(
		"session_id", "context_id", "", []byte(network[0].ID()), nil, nil)
	require.NoError(t, err)
	conn10, err := NewConn(1, session10)
	require.NoError(t, err)

	num := 10000
	payload1 := []byte("Message in a bottle")
	payload2 := []byte("bottle in a message")

	errCh := make(chan error, 4*num)
	var wg sync.WaitGroup
	wg.Go(func() { send(conn01, num, payload1, errCh) })
	wg.Go(func() { receive(conn10, num, payload1, errCh) })
	wg.Go(func() { send(conn10, num, payload2, errCh) })
	wg.Go(func() { receive(conn01, num, payload2, errCh) })
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}

func receive(session Conn, num int, payload []byte, errCh chan<- error) {
	for i := 0; i < num; i++ {
		msg := make([]byte, len(payload))
		n, err := session.Read(msg)
		if err != nil {
			errCh <- err
			continue
		}
		if n != len(payload) {
			errCh <- errors.Errorf("expected length %d, got %d", len(payload), n)
			continue
		}
		if !bytes.Equal(payload, msg) {
			errCh <- errors.Errorf("expected %q, got %q", payload, msg)
		}
	}
}

func send(session Conn, num int, payload []byte, errCh chan<- error) {
	for i := 0; i < num; i++ {
		n, err := session.Write(payload)
		if err != nil {
			errCh <- err
			continue
		}
		if n != len(payload) {
			errCh <- errors.Errorf("expected to write %d bytes, wrote %d", len(payload), n)
		}
	}
}

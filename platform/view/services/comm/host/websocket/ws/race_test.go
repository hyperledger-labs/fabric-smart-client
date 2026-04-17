/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type raceConn struct {
	read chan resultMsg
}

type resultMsg struct {
	msgType int
	msg     []byte
	err     error
}

func (c *raceConn) ReadMessage() (int, []byte, error) {
	r, ok := <-c.read
	if !ok {
		return 0, nil, io.EOF
	}
	return r.msgType, r.msg, r.err
}

func (c *raceConn) WriteMessage(messageType int, data []byte) error { return nil }
func (c *raceConn) Close() error {
	return nil
}

func TestReadRace(t *testing.T) { //nolint:paralleltest
	p := make([]byte, 100)
	for i := 0; i < 100; i++ {
		readChan := make(chan resultMsg, 2)
		conn := &raceConn{read: readChan}
		// Send one message
		readChan <- resultMsg{msgType: 0, msg: []byte("hello"), err: nil}
		// Then EOF
		close(readChan)

		s := NewWSStream(conn, t.Context(), host.StreamInfo{})

		// Read "hello"
		_, err := s.Read(p)
		require.NoError(t, err)

		// Now Read should return io.EOF
		// Give it a tiny bit of time for readMessages to deliver streamEOF and Close()
		time.Sleep(1 * time.Millisecond)

		_, err = s.Read(p)
		if errors.Is(err, context.Canceled) {
			t.Fatalf("REPRODUCED: Iteration %d: Got context.Canceled instead of io.EOF", i)
		}
		require.Equal(t, io.EOF, err)
		_ = s.Close()
	}
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	io2 "io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/ws"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io"
)

const testMaxMessageSize = 10 * 1024 * 1024

func newMockStream(conn *mockConn) host.P2PStream {
	return ws.NewWSStream(conn, context.Background(), host.StreamInfo{})
}

type mockConn struct {
	written chan []byte
	read    chan []byte

	once sync.Once
}

func (c *mockConn) ReadMessage() (int, []byte, error) {
	return 0, <-c.read, nil
}

func (c *mockConn) WriteMessage(_ int, data []byte) error {
	c.written <- data
	return nil
}

func (c *mockConn) Close() error {
	c.once.Do(func() {
		close(c.read)
		close(c.written)
	})
	return nil
}

func (c *mockConn) ReadValue(message proto.Message) error {
	data, err := proto.Marshal(message)
	if err != nil {
		return err
	}
	p := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(p, uint64(len(data)))
	c.read <- p[:n]
	c.read <- data
	return nil
}

func (c *mockConn) WrittenValues() <-chan []byte {
	return c.written
}

func TestWriter(t *testing.T) { //nolint:paralleltest
	// let check that at the end of this test all our go routines are stopped
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	conn := &mockConn{
		written: make(chan []byte, 100),
		read:    make(chan []byte, 100),
	}
	stream := newMockStream(conn)
	w := io.NewVarintProtoWriter(stream, 1024*1024)

	input := []proto.Message{
		messageOfSize(12),
		messageOfSize(15),
	}
	for _, message := range input {
		require.NoError(t, w.WriteMsg(message))
	}
	require.NoError(t, stream.Close())

	output := make([][]byte, 0, len(input))
	m := sync.RWMutex{}
	go func() {
		for written := range conn.WrittenValues() {
			m.Lock()
			output = append(output, written)
			m.Unlock()
		}
	}()

	require.Eventually(t, func() bool {
		m.RLock()
		defer m.RUnlock()
		fmt.Printf("input: %v\noutput: %v\n\n", input, output)
		return len(input) == len(output)
	}, 5*time.Second, time.Second)
}

func TestReader(t *testing.T) { //nolint:paralleltest
	// let check that at the end of this test all our go routines are stopped
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	conn := &mockConn{
		written: make(chan []byte, 100),
		read:    make(chan []byte, 100),
	}
	stream := newMockStream(conn)
	r := io.NewVarintProtoReader(stream, 2, testMaxMessageSize)

	input := []proto.Message{
		messageOfSize(12),
		messageOfSize(16),
		messageOfSize(14),
		messageOfSize(1400000),
	}
	for _, message := range input {
		require.NoError(t, conn.ReadValue(message))
	}
	wg := sync.WaitGroup{}

	wg.Go(func() {
		for _, in := range input {
			read := &comm.ViewPacket{}
			assert.NoError(t, r.ReadMsg(read)) //nolint:testifylint // runs inside wg.Go and a loop; require.FailNow is unsafe outside the test goroutine
			assert.True(t, proto.Equal(in, read))
		}
	})
	wg.Wait()

	require.NoError(t, stream.Close())
}

func messageOfSize(size int) proto.Message {
	if size < 2 {
		panic("too small message")
	}
	return &comm.ViewPacket{Payload: bytes.Repeat([]byte{1}, size-2)}
}

func TestStreamAccessorsAndReadAfterClose(t *testing.T) { //nolint:paralleltest
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	conn := &mockConn{written: make(chan []byte, 1), read: make(chan []byte, 1)}
	info := host.StreamInfo{RemotePeerID: "peer", RemotePeerAddress: "127.0.0.1:1", ContextID: "ctx"}
	stream := ws.NewWSStream(conn, context.Background(), info)
	require.Equal(t, info.RemotePeerAddress, stream.RemotePeerAddress())
	require.NoError(t, stream.Context().Err())

	conn.read <- []byte("hello")
	buf := make([]byte, 3)
	n, err := stream.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "hel", string(buf[:n]))

	require.NoError(t, stream.Close())
	require.ErrorIs(t, stream.Context().Err(), context.Canceled)
	// the leftover of a value already taken off the channel is still returned
	n, err = stream.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "lo", string(buf[:n]))

	_, err = stream.Read(buf)
	require.True(t, errors.Is(err, io2.EOF) || errors.Is(err, context.Canceled), "got %v", err)
}

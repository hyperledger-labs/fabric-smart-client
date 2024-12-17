/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/gogo/protobuf/io"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/websocket"
	"github.com/stretchr/testify/assert"
)

func newMockStream(conn *mockConn) host.P2PStream {
	return websocket.NewWSStream(conn, context.Background(), host.StreamInfo{})
}

type mockConn struct {
	written chan []byte
	read    chan []byte
}

func (c *mockConn) ReadMessage() (int, []byte, error) {
	return 0, <-c.read, nil
}
func (c *mockConn) WriteMessage(_ int, data []byte) error {
	c.written <- data
	return nil
}
func (c *mockConn) Close() error {
	close(c.read)
	close(c.written)
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

func TestWriter(t *testing.T) {
	conn := &mockConn{
		written: make(chan []byte, 100),
		read:    make(chan []byte, 100),
	}
	stream := newMockStream(conn)
	w := io.NewDelimitedWriter(stream)

	input := []proto.Message{
		messageOfSize(12),
		messageOfSize(15),
	}
	for _, message := range input {
		assert.NoError(t, w.WriteMsg(message))
	}
	assert.NoError(t, stream.Close())

	output := make([][]byte, 0, len(input))
	m := sync.RWMutex{}
	go func() {
		for written := range conn.WrittenValues() {
			m.Lock()
			output = append(output, written)
			m.Unlock()
		}
	}()

	assert.Eventually(t, func() bool {
		m.RLock()
		defer m.RUnlock()
		return len(input) == len(output)
	}, 5*time.Second, time.Second)
}

func TestReader(t *testing.T) {
	conn := &mockConn{
		written: make(chan []byte, 100),
		read:    make(chan []byte, 100),
	}
	stream := newMockStream(conn)
	r := comm.NewDelimitedReader(stream, 2)

	input := []proto.Message{
		messageOfSize(12),
		messageOfSize(16),
		messageOfSize(14),
		messageOfSize(1400000),
	}
	for _, message := range input {
		assert.NoError(t, conn.ReadValue(message))
	}
	wg := sync.WaitGroup{}
	wg.Add(len(input))

	output := make([]*comm.ViewPacket, 0, len(input))
	m := sync.RWMutex{}
	go func() {
		for {
			read := &comm.ViewPacket{}
			assert.NoError(t, r.ReadMsg(read))
			m.Lock()
			output = append(output, read)
			m.Unlock()
			wg.Done()
		}
	}()
	wg.Wait()

	assert.Equal(t, len(input), len(output))
}

func messageOfSize(size int) proto.Message {
	if size < 2 {
		panic("too small message")
	}
	return &comm.ViewPacket{Payload: bytes.Repeat([]byte{1}, size-2)}
}

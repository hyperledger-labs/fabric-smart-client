/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	p2pproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/grpc/protos"
)

type mockPacketStream struct {
	ctx     context.Context
	packets chan *p2pproto.StreamPacket
}

func (m *mockPacketStream) Send(*p2pproto.StreamPacket) error {
	return nil
}

func (m *mockPacketStream) Recv() (*p2pproto.StreamPacket, error) {
	packet, ok := <-m.packets
	if !ok {
		return nil, io.EOF
	}
	return packet, nil
}

func (m *mockPacketStream) Context() context.Context {
	return m.ctx
}

func TestReadReturnsEOFInsteadOfContextCanceledAfterPeerClose(t *testing.T) { //nolint:paralleltest
	raw := &mockPacketStream{
		ctx:     t.Context(),
		packets: make(chan *p2pproto.StreamPacket, 1),
	}
	raw.packets <- &p2pproto.StreamPacket{
		Body: &p2pproto.StreamPacket_Payload{Payload: []byte("hello")},
	}
	close(raw.packets)

	s := NewGRPCStream(raw, nil, t.Context(), host2.StreamInfo{})

	buf := make([]byte, 16)
	n, err := s.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "hello", string(buf[:n]))

	time.Sleep(1 * time.Millisecond)

	_, err = s.Read(buf)
	require.Equal(t, io.EOF, err)
}

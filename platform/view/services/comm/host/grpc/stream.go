/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"io"
	"sync"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	p2pproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/grpc/protos"
)

type grpcPacketStream interface {
	Send(*p2pproto.StreamPacket) error
	Recv() (*p2pproto.StreamPacket, error)
	Context() context.Context
}

type closeSender interface {
	CloseSend() error
}

type streamResult struct {
	value []byte
	err   error
}

type stream struct {
	ctx            context.Context
	cancel         context.CancelFunc
	raw            grpcPacketStream
	connCloser     io.Closer
	accumulator    *delimitedReader
	reads          chan streamResult
	closeOnce      sync.Once
	closeReadsOnce sync.Once
	info           host2.StreamInfo

	mu           sync.Mutex
	isClosed     bool
	readLeftover []byte
}

func NewGRPCStream(raw grpcPacketStream, connCloser io.Closer, parent context.Context, info host2.StreamInfo) *stream {
	if parent == nil {
		parent = raw.Context()
	}
	ctx, cancel := context.WithCancel(parent)
	s := &stream{
		ctx:          ctx,
		cancel:       cancel,
		raw:          raw,
		connCloser:   connCloser,
		accumulator:  newDelimitedReader(),
		reads:        make(chan streamResult, 100),
		readLeftover: []byte{},
		info:         info,
	}
	go s.readPackets()
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	return s
}

func (s *stream) readPackets() {
	defer func() { _ = s.Close() }()
	defer s.closeReadsOnce.Do(func() { close(s.reads) })
	for {
		packet, err := s.raw.Recv()
		if err != nil {
			s.deliver(streamResult{err: err})
			return
		}
		payload, ok := packet.Body.(*p2pproto.StreamPacket_Payload)
		if !ok {
			s.deliver(streamResult{err: io.ErrUnexpectedEOF})
			return
		}
		s.deliver(streamResult{value: payload.Payload})
	}
}

func (s *stream) deliver(r streamResult) {
	s.mu.Lock()
	if s.isClosed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	select {
	case s.reads <- r:
	case <-s.ctx.Done():
	}
}

func (s *stream) RemotePeerID() host2.PeerID {
	return s.info.RemotePeerID
}

func (s *stream) RemotePeerAddress() host2.PeerIPAddress {
	return s.info.RemotePeerAddress
}

func (s *stream) Hash() host2.StreamHash {
	return StreamHash(s.info)
}

func (s *stream) Context() context.Context {
	return s.ctx
}

func (s *stream) Read(p []byte) (int, error) {
	if len(s.readLeftover) > 0 {
		n := copy(p, s.readLeftover)
		s.readLeftover = s.readLeftover[n:]
		return n, nil
	}

	select {
	case r, ok := <-s.reads:
		if !ok {
			return 0, io.EOF
		}
		if r.err != nil {
			return 0, r.err
		}
		s.readLeftover = r.value
		return s.Read(p)
	case <-s.ctx.Done():
		select {
		case r, ok := <-s.reads:
			if !ok {
				return 0, io.EOF
			}
			if r.err != nil {
				return 0, r.err
			}
			s.readLeftover = r.value
			return s.Read(p)
		default:
		}
		if len(s.readLeftover) > 0 {
			n := copy(p, s.readLeftover)
			s.readLeftover = s.readLeftover[n:]
			return n, nil
		}
		return 0, s.ctx.Err()
	}
}

func (s *stream) Write(p []byte) (int, error) {
	n, err := s.accumulator.Read(p)
	if err != nil {
		return 0, err
	}
	content := s.accumulator.Flush()
	if content == nil {
		return n, nil
	}
	if err := s.raw.Send(&p2pproto.StreamPacket{
		Body: &p2pproto.StreamPacket_Payload{Payload: content},
	}); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *stream) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.isClosed = true
		s.mu.Unlock()
		s.cancel()
		if closeable, ok := s.raw.(closeSender); ok {
			err = closeable.CloseSend()
		}
		if s.connCloser != nil {
			if cerr := s.connCloser.Close(); err == nil {
				err = cerr
			}
		}
	})
	return err
}

var _ host2.P2PStream = (*stream)(nil)

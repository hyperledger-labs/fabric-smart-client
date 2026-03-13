/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"go.uber.org/zap/zapcore"
)

type connection interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

var streamEOF = result{err: io.EOF}

type stream struct {
	ctx         context.Context
	conn        connection
	cancel      context.CancelFunc
	accumulator *delimitedReader
	reads       chan result
	closeOnce   sync.Once
	info        host2.StreamInfo

	mu       sync.Mutex
	isClosed bool

	// Sometimes Read doesn't read the whole value that comes from the reads channel, because of buffering.
	// In this case, we store what was not read in readLeftover, and on the next read, we attempt first to check whether
	// there were any leftover bytes from the previous value of the reads.
	// If not, then we consume the next value from the reads channel
	readLeftover []byte
}

func NewWSStream(conn connection, ctx context.Context, info host2.StreamInfo) *stream {
	ctx, cancel := context.WithCancel(ctx)
	s := &stream{
		ctx:          ctx,
		conn:         conn,
		cancel:       cancel,
		accumulator:  newDelimitedReader(),
		reads:        make(chan result, 100),
		readLeftover: []byte{},
		info:         info,
	}
	go s.readMessages(ctx)
	go func() {
		<-ctx.Done()
		if err := s.Close(); err != nil {
			logger.Debugf("error closing stream on context cancellation [%s]: [%s]", s.Hash(), err)
		}
	}()
	return s
}

func (s *stream) readMessages(ctx context.Context) {
	defer func() { _ = s.Close() }()
	for {
		select {
		case <-ctx.Done():
			logger.Debugf("Context for stream [%s] closed by us. Error: %v", s.Hash(), ctx.Err())
			s.deliver(streamEOF)
			return
		default:
			if c, ok := s.conn.(*websocket.Conn); ok {
				_ = c.SetReadDeadline(time.Now().Add(readTimeout))
			}
			_, msg, err := s.conn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
					logger.Debugf("Websocket connection closed unexpectedly: %v", err)
				}
				s.deliver(streamEOF)
				return
			}
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Read message of length [%d] on [%s]", len(msg), s.Hash())
			}
			s.deliver(result{value: msg, err: err})
		}
	}
}

func (s *stream) deliver(r result) {
	s.mu.Lock()
	if s.isClosed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	select {
	case s.reads <- r:
	case <-s.ctx.Done():
		logger.Debugf("dropping message for stream [%s] because context is done", s.Hash())
	case <-time.After(DefaultStreamTimeout):
		logger.Errorf("dropping message for stream [%s] after %s timeout, closing stream", s.Hash(), DefaultStreamTimeout)
		_ = s.Close()
	}
}

func (s *stream) RemotePeerID() host2.PeerID {
	return s.info.RemotePeerID
}

func (s *stream) RemotePeerAddress() host2.PeerIPAddress {
	return s.info.RemotePeerAddress
}

func (s *stream) ContextID() string {
	return s.info.ContextID
}

func (s *stream) Hash() host2.StreamHash {
	return rest.StreamHash(s.info)
}

type result struct {
	value []byte
	err   error
}

func (s *stream) Read(p []byte) (int, error) {
	if len(s.readLeftover) == 0 {
		// The previous value from the channel has been read completely
		logger.Debugf("[%s@%s] waits to read from channel...", s.info.RemotePeerID, s.info.RemotePeerAddress)
		select {
		case r, ok := <-s.reads:
			if !ok {
				return 0, io.EOF
			}
			if r.err != nil {
				logger.Debugf("error occurred while [%s] was reading: %v", s.info.RemotePeerID, r.err)
				return 0, r.err
			}
			s.readLeftover = r.value
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		}
	} else {
		logger.Debugf("Reading from remaining %d bytes from previous value", len(s.readLeftover))
	}
	n := copy(p, s.readLeftover)
	s.readLeftover = s.readLeftover[n:]
	logger.Debugf("[%s@%s] copied %d bytes, remaining %d bytes", s.info.RemotePeerID, s.info.RemotePeerAddress, n, len(s.readLeftover))
	return n, nil
}

func (s *stream) Write(p []byte) (int, error) {
	n, err := s.accumulator.Read(p)
	if err != nil {
		return 0, err
	}
	content := s.accumulator.Flush()
	if content == nil {
		logger.Debugf("Wrote to [%s@%s], but message not ready yet.", s.info.RemotePeerID, s.info.RemotePeerAddress)
		return n, nil
	}
	logger.Debugf("Ready to send to [%s@%s]: [%s]", s.info.RemotePeerID, s.info.RemotePeerAddress, logging.Base64(content))
	if c, ok := s.conn.(*websocket.Conn); ok {
		_ = c.SetWriteDeadline(time.Now().Add(writeTimeout))
	}
	if err := s.conn.WriteMessage(websocket.BinaryMessage, content); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *stream) Close() error {
	var err error
	s.closeOnce.Do(func() {
		logger.Debugf("Close connection for context [%s]", s.ContextID())
		s.cancel()
		err = s.conn.Close()
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.isClosed {
			s.isClosed = true
		}
	})
	return err
}

func (s *stream) Context() context.Context {
	return s.ctx
}

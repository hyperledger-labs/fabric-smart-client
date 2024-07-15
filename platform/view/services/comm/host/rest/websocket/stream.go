/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"io"

	"github.com/gorilla/websocket"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
)

type connection interface {
	ID() SubConnId
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

type result struct {
	value []byte
	err   error
}

var streamEOF = result{err: io.EOF}

type stream struct {
	ctx         context.Context
	conn        connection
	cancel      context.CancelFunc
	accumulator *delimitedReader
	reads       chan result
	info        host2.StreamInfo

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
	return s
}

func (s *stream) readMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Warnf("Context for stream [%s][%s] closed by us. Error: %w", s.conn.ID(), s.Hash(), ctx.Err())
			s.reads <- streamEOF
			return
		default:
			_, msg, err := s.conn.ReadMessage()
			if err != nil && websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
				logger.Warnf("Websocket connection closed unexpectedly")
				s.reads <- streamEOF
				s.Close()
				return
			}
			logger.Debugf("Read message on [%s][%s]: [%s]. Error encountered: %w", s.conn.ID(), s.Hash(), string(msg), err)
			s.reads <- result{value: msg, err: err}
		}
	}
}

func (s *stream) RemotePeerID() host2.PeerID {
	return s.info.RemotePeerID
}

func (s *stream) RemotePeerAddress() host2.PeerIPAddress {
	return s.info.RemotePeerAddress
}

func (s *stream) Hash() host2.StreamHash {
	return rest.StreamHash(s.info)
}

func (s *stream) Read(p []byte) (int, error) {
	if len(s.readLeftover) == 0 {
		// The previous value from the channel has been read completely
		logger.Debugf("[%s][%s@%s] waits to read from channel...", s.conn.ID(), s.info.RemotePeerID, s.info.RemotePeerAddress)
		r := <-s.reads
		if r.err != nil {
			logger.Errorf("error occurred while [%s] was reading: %v", s.info.RemotePeerID, r.err)
			return 0, r.err
		}
		s.readLeftover = r.value
	} else {
		logger.Debugf("Reading from remaining %d bytes from previous value", len(s.readLeftover))
	}
	n := copy(p, s.readLeftover)
	s.readLeftover = s.readLeftover[n:]
	logger.Debugf("[%s][%s@%s] copied %d bytes, remaining %d bytes", s.conn.ID(), s.info.RemotePeerID, s.info.RemotePeerAddress, n, len(s.readLeftover))
	return n, nil
}

func (s *stream) Write(p []byte) (int, error) {
	n, err := s.accumulator.Read(p)
	if err != nil {
		return 0, err
	}
	content := s.accumulator.Flush()
	if content == nil {
		logger.Debugf("Wrote to [%s][%s@%s], but message not ready yet.", s.conn.ID(), s.info.RemotePeerID, s.info.RemotePeerAddress)
		return n, nil
	}
	logger.Debugf("Ready to send to [%s][%s@%s]: [%s]", s.conn.ID(), s.info.RemotePeerID, s.info.RemotePeerAddress, content)
	if err := s.conn.WriteMessage(websocket.TextMessage, content); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *stream) Close() error {
	s.cancel()
	return s.conn.Close()
}

func (s *stream) Context() context.Context {
	return s.ctx
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web"
	"github.com/pkg/errors"
)

var streamEOF = result{err: io.EOF}

type stream struct {
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

// StreamMeta is the first message sent from the websocket client to transmit metadata information
type StreamMeta struct {
	SessionID string       `json:"session_id"`
	ContextID string       `json:"context_id"`
	PeerID    host2.PeerID `json:"peer_id"`
}

var schemes = map[bool]string{
	true:  "wss",
	false: "ws",
}

type connection interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

func newClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (*stream, error) {
	logger.Infof("Creating new stream from [%s] to [%s@%s]...", src, info.RemotePeerID, info.RemotePeerAddress)
	tlsEnabled := config.InsecureSkipVerify || config.RootCAs != nil
	url := url.URL{Scheme: schemes[tlsEnabled], Host: info.RemotePeerAddress, Path: "/p2p"}
	// We use the background context instead of passing the existing context,
	// because the net/http server doesn't monitor connections upgraded to WebSocket.
	// Hence, when the connection is lost, the context will not be cancelled.
	conn, err := web2.OpenWSClientConn(url.String(), config)
	logger.Infof("Successfully connected to websocket")
	if err != nil {
		logger.Errorf("Dial failed: %s\n", err.Error())
		return nil, err
	}
	meta := StreamMeta{
		ContextID: info.ContextID,
		SessionID: info.SessionID,
		PeerID:    src,
	}
	err = conn.WriteJSON(&meta)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to send meta message")
	}
	logger.Infof("Stream opened to [%s@%s]", info.RemotePeerID, info.RemotePeerAddress)
	return NewWSStream(conn, ctx, info), nil
}

func newServerStream(writer http.ResponseWriter, request *http.Request) (*stream, error) {
	conn, err := web.OpenWSServerConn(writer, request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open websocket")
	}
	logger.Infof("Successfully opened server-side websocket")

	var meta StreamMeta
	if err := conn.ReadJSON(&meta); err != nil {
		return nil, errors.Wrapf(err, "failed to read meta info")
	}
	logger.Infof("Read meta info: %v", meta)
	// Propagating the request context will not make a difference (see comment in newClientStream)
	return NewWSStream(conn, context.Background(), host2.StreamInfo{
		RemotePeerID:      meta.PeerID,
		RemotePeerAddress: request.RemoteAddr,
		ContextID:         meta.ContextID,
		SessionID:         meta.SessionID,
	}), nil
}

func NewWSStream(conn connection, ctx context.Context, info host2.StreamInfo) *stream {
	ctx, cancel := context.WithCancel(ctx)
	s := &stream{
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
			logger.Warnf("Context for stream [%s] closed by us. Error: %w", s.Hash(), ctx.Err())
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
			logger.Debugf("Read message on [%s]: [%s]. Error encountered: %w", s.Hash(), string(msg), err)
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
	return streamHash(s.info)
}

func streamHash(info host2.StreamInfo) host2.StreamHash {
	return info.RemotePeerAddress
}

type result struct {
	value []byte
	err   error
}

func (s *stream) Read(p []byte) (int, error) {
	if len(s.readLeftover) == 0 {
		// The previous value from the channel has been read completely
		logger.Debugf("[%s@%s] waits to read from channel...", s.info.RemotePeerID, s.info.RemotePeerAddress)
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
	logger.Debugf("Ready to send to [%s@%s]: [%s]", s.info.RemotePeerID, s.info.RemotePeerAddress, content)
	if err := s.conn.WriteMessage(websocket.TextMessage, content); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *stream) Close() error {
	s.cancel()
	return s.conn.Close()
}

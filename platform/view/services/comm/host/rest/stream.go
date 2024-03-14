/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/websocket"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web"
	"github.com/pkg/errors"
)

const streamIDLength = 128

type bufferedReader struct {
	bytes  []byte
	length int
}

func newBufferedReader() *bufferedReader {
	return &bufferedReader{
		bytes: []byte{},
	}
}

func (r *bufferedReader) Read(p []byte) []byte {

	r.bytes = append(r.bytes, p...)
	if r.length <= 0 {
		length, err := binary.ReadUvarint(bytes.NewReader(p))
		if err != nil {
			panic("failed to read length [" + string(p) + "]: " + err.Error())
		}
		r.length = int(length) + len(p)
		logger.Infof("Reading only size: %d [%v]", r.length, len(p))
		return nil
	}

	if len(r.bytes) > r.length {
		panic("too many elements added [" + strconv.Itoa(len(r.bytes)) + "/" + strconv.Itoa(r.length) + "]")
	}

	logger.Infof("appended [%s] and expecting %d in total. current length: %d", string(p), r.length, len(r.bytes))
	if len(r.bytes) < r.length {
		return nil
	}
	defer func() {
		r.bytes = []byte{}
		r.length = 0
	}()
	return bytes.Clone(r.bytes)
}

type wsStream struct {
	conn        *websocket.Conn
	accumulator *bufferedReader
	streamID    string
	peerID      host2.PeerID
	peerAddress host2.PeerIPAddress
	reads       <-chan result

	// Sometimes Read doesn't read the whole value that comes from the reads channel, because of buffering.
	// In this case, we store what was not read in readLeftover, and on the next read, we attempt first to check whether
	// there were any leftover bytes from the previous value of the reads.
	// If not, then we consume the next value from the reads channel
	readLeftover []byte
}

type StreamInfo struct {
	StreamID []byte       `json:"stream_id"`
	PeerID   host2.PeerID `json:"peer_id"`
}

var schemes = map[bool]string{
	true:  "wss",
	false: "ws",
}

func newClientStream(peerAddress host2.PeerIPAddress, src, dst host2.PeerID, config *tls.Config) (*wsStream, error) {
	logger.Infof("Creating new stream from [%s] to [%s@%s]...", src, dst, peerAddress)
	tlsEnabled := config.InsecureSkipVerify || config.RootCAs != nil
	url := url.URL{Scheme: schemes[tlsEnabled], Host: peerAddress, Path: "/p2p"}
	streamID := make([]byte, streamIDLength)
	if _, err := rand.Read(streamID); err != nil {
		return nil, err
	}

	conn, err := web2.OpenWSClientConn(url.String(), config)
	logger.Infof("Successfully connected to websocket")
	if err != nil {
		logger.Errorf("Dial failed: %s\n", err.Error())
		return nil, err
	}
	err = conn.WriteJSON(&StreamInfo{
		StreamID: streamID,
		PeerID:   src,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to send init message")
	}
	logger.Infof("Stream opened to [%s@%s]", dst, peerAddress)
	return newWSStream(conn, streamID, dst, peerAddress), nil
}

func newServerStream(writer http.ResponseWriter, request *http.Request) (*wsStream, error) {
	conn, err := web.OpenWSServerConn(writer, request)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open websocket")
	}
	logger.Infof("Successfully opened server-side websocket")

	var info StreamInfo
	if err := conn.ReadJSON(&info); err != nil {
		return nil, errors.Wrapf(err, "failed to read init info")
	}
	logger.Infof("Read init info: %v", info)
	return newWSStream(conn, info.StreamID, info.PeerID, request.RemoteAddr), nil
}

func newWSStream(conn *websocket.Conn, streamID []byte, peerID host2.PeerID, peerAddress host2.PeerIPAddress) *wsStream {
	reads := make(chan result, 100)
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			logger.Infof("Received message on [%s@%s]: [%s], errors: [%s]", peerID, peerAddress, string(msg), err)
			reads <- result{
				value: msg,
				err:   err,
			}
		}
	}()

	return &wsStream{
		conn:         conn,
		streamID:     string(streamID),
		peerID:       peerID,
		peerAddress:  peerAddress,
		accumulator:  newBufferedReader(),
		reads:        reads,
		readLeftover: []byte{},
	}
}

func (s *wsStream) RemotePeerID() host2.PeerID {
	return s.peerID
}

func (s *wsStream) RemotePeerAddress() host2.PeerIPAddress {
	return s.peerAddress
}

type result struct {
	value []byte
	err   error
}

func (s *wsStream) Read(p []byte) (int, error) {
	if len(s.readLeftover) == 0 {
		// The previous value from the channel has been read completely
		logger.Debugf("[%s@%s] waits to read from channel...", s.peerID, s.peerAddress)
		r := <-s.reads
		if r.err != nil {
			logger.Errorf("error occurred while [%s] was reading: %v", s.peerID, r.err)
			return 0, r.err
		}
		s.readLeftover = r.value
	} else {
		logger.Debugf("Reading from remaining %d bytes from previous value", len(s.readLeftover))
	}
	n := copy(p, s.readLeftover)
	s.readLeftover = s.readLeftover[n:]
	logger.Debugf("[%s@%s] copied %d bytes, remaining %d bytes", s.peerID, s.peerAddress, n, len(s.readLeftover))
	return n, nil
}

func (s *wsStream) Write(p []byte) (int, error) {
	content := s.accumulator.Read(p)
	if content == nil {
		logger.Debugf("Wrote to [%s@%s], but message not ready yet (%d/%d received): [%s]", s.peerID, s.peerAddress, len(s.accumulator.bytes), s.accumulator.length, string(s.accumulator.bytes))
		return len(p), nil
	}
	logger.Debugf("Ready to send to [%s@%s]: [%s]", s.peerID, s.peerAddress, content)
	if err := s.conn.WriteMessage(websocket.TextMessage, content); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *wsStream) Close() error {
	return s.conn.Close()
}

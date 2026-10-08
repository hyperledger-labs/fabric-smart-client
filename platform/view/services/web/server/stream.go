/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

type Input struct {
	Raw []byte
}

type Output struct {
	Raw []byte
}

type WSStream struct {
	ws *websocket.Conn
}

// maxMessageSize bounds both a websocket message and an HTTP request body.
const maxMessageSize = 10 * 1024 * 1024

const (
	// closeTimeout bounds writing the close frame.
	closeTimeout = time.Second
	// drainTimeout bounds discarding unread input while waiting for the peer's close frame.
	drainTimeout = 100 * time.Millisecond
	// maxCloseReason is the longest reason a close frame carries next to its 2-byte code.
	maxCloseReason = 123
)

func OpenWSServerConn(writer http.ResponseWriter, request *http.Request) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     sameOriginOrNonBrowser,
	}
	conn, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		logger.Errorf("Failed to upgrade connection to websocket from [%s]: %s", request.RemoteAddr, err.Error())
		return nil, err
	}
	conn.SetReadLimit(maxMessageSize)
	return conn, nil
}

func sameOriginOrNonBrowser(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		// Non-browser clients generally don't set Origin.
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, request.Host)
}

func NewWSStream(writer http.ResponseWriter, request *http.Request) (*WSStream, error) {
	ws, err := OpenWSServerConn(writer, request)
	if err != nil {
		return nil, err
	}
	logger.Infof("Upgraded to web socket")
	return &WSStream{ws: ws}, nil
}

func (c *WSStream) Recv(p any) error {
	message, err := c.Read()
	if err != nil {
		return err
	}
	return json.Unmarshal(message, p)
}

func (c *WSStream) Send(p any) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return c.Write(data)
}

func (c *WSStream) Read() ([]byte, error) {
	_, message, err := c.ws.ReadMessage()
	if err != nil {
		logger.Errorf("error receiving message: %v", err)
		return nil, err
	}
	logger.Debugf("received message: %s", message)
	return message, nil
}

func (c *WSStream) Write(message []byte) error {
	logger.Debugf("sending message: %s", message)
	err := c.ws.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		logger.Errorf("error writing message: %v", err)
	}
	return err
}

// Close closes the stream with a normal-closure frame; see CloseWithError.
func (c *WSStream) Close() error {
	return c.CloseWithError(nil)
}

// CloseWithError sends a close frame, discards incoming messages until the peer answers or
// drainTimeout elapses, and closes the connection. The frame carries CloseNormalClosure when
// cause is nil, and CloseInternalServerErr with cause's text, truncated to fit, otherwise.
// Closing with unread input would send a TCP reset, which discards output still in flight, so
// the input is discarded even when the close frame cannot be sent. CloseWithError reads, so it
// must not run concurrently with Read or Recv.
func (c *WSStream) CloseWithError(cause error) error {
	logger.Debugf("closing web socket")
	code, reason := websocket.CloseNormalClosure, ""
	if cause != nil {
		code, reason = websocket.CloseInternalServerErr, closeReason(cause.Error())
	}
	closeFrame := websocket.FormatCloseMessage(code, reason)
	if err := c.ws.WriteControl(websocket.CloseMessage, closeFrame, time.Now().Add(closeTimeout)); err != nil {
		logger.Warnf("failed sending close frame: %v", err)
	}
	_ = c.ws.SetReadDeadline(time.Now().Add(drainTimeout))
	for {
		if _, _, err := c.ws.NextReader(); err != nil {
			break
		}
	}
	err := c.ws.Close()
	if err != nil {
		logger.Errorf("error closing web socket: %v", err)
	}
	return err
}

// closeReason drops invalid UTF-8 from s, which a peer rejects in a close reason, and truncates
// it to maxCloseReason bytes on a rune boundary.
func closeReason(s string) string {
	s = strings.ToValidUTF8(s, "")
	if len(s) <= maxCloseReason {
		return s
	}
	s = s[:maxCloseReason]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func (c *WSStream) ReadInput() ([]byte, error) {
	input := &Input{}
	if err := c.Recv(input); err != nil {
		return nil, err
	}
	return input.Raw, nil
}

func (c *WSStream) WriteResult(raw []byte) error {
	return c.Send(&Output{Raw: raw})
}

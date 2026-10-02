/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

type Input = server.Input

type Output = server.Output

type WSStream struct {
	conn *websocket.Conn
}

const (
	maxMessageSize          = 10 * 1024 * 1024
	DefaultHandshakeTimeout = 30 * time.Second
)

// OpenWSClientConn establishes a websocket connection to the given URL using DefaultHandshakeTimeout.
func OpenWSClientConn(url string, config *tls.Config) (*websocket.Conn, error) {
	return OpenWSClientConnContext(context.Background(), url, config) //nolint:contextcheck // non-context convenience wrapper defaults to context.Background()
}

// OpenWSClientConnContext establishes a websocket connection to the given URL with the provided context and DefaultHandshakeTimeout.
func OpenWSClientConnContext(ctx context.Context, url string, config *tls.Config) (*websocket.Conn, error) { //nolint:contextcheck // documented nil-ctx fallback below (nil is treated as context.Background), not an ignored inherited context
	if ctx == nil {
		ctx = context.Background()
	}
	dialer := &websocket.Dialer{
		TLSClientConfig:  config,
		HandshakeTimeout: DefaultHandshakeTimeout,
	}
	ws, resp, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		logger.Errorf("Failed to establish websocket connection to [%s]: %s", url, err.Error())
		return nil, err
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	ws.SetReadLimit(maxMessageSize)
	return ws, nil
}

func NewWSStream(url string, config *tls.Config) (*WSStream, error) {
	logger.Debugf("connecting to %s", url)
	ws, err := OpenWSClientConn(url, config)
	if err != nil {
		logger.Errorf("dial [%s] failed: %s\n", url, err.Error())
		return nil, err
	}
	logger.Debug("successfully connected to websocket")
	return &WSStream{conn: ws}, nil
}

func (c *WSStream) Send(v any) error {
	return c.conn.WriteJSON(v)
}

func (c *WSStream) Recv(v any) error {
	return c.conn.ReadJSON(v)
}

func (c *WSStream) Close() error {
	return c.conn.Close()
}

func (c *WSStream) Result() ([]byte, error) {
	output := &Output{}
	if err := c.Recv(output); err != nil {
		return nil, err
	}
	return output.Raw, nil
}

func (c *WSStream) SendInput(in []byte) error {
	return c.Send(&Input{Raw: in})
}

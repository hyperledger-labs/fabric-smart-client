/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"crypto/tls"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

type Input = server.Input

type Output = server.Output

type WSStream struct {
	conn *websocket.Conn
}

func OpenWSClientConn(url string, config *tls.Config) (*websocket.Conn, error) {
	dialer := &websocket.Dialer{TLSClientConfig: config}
	ws, _, err := dialer.Dial(url, nil)
	return ws, err
}

func NewWSStream(url string, config *tls.Config) (*WSStream, error) {
	logger.Debugf("Connecting to %s", url)
	ws, err := OpenWSClientConn(url, config)
	logger.Debugf("Successfully connected to websocket")
	if err != nil {
		logger.Errorf("Dial failed: %s\n", err.Error())
		return nil, err
	}
	return &WSStream{conn: ws}, nil
}

func (c *WSStream) Send(v interface{}) error {
	return c.conn.WriteJSON(v)
}

func (c *WSStream) Recv(v interface{}) error {
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

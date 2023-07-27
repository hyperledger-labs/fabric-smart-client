/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"crypto/tls"

	"github.com/gorilla/websocket"
)

func NewWsStream(url string, config *tls.Config) (*WsServiceStream, error) {
	logger.Debugf("Connecting to %s", url)
	dialer := &websocket.Dialer{TLSClientConfig: config}
	ws, _, err := dialer.Dial(url, nil)
	logger.Infof("Successfully connected to websocket")
	if err != nil {
		logger.Errorf("Dial failed: %s\n", err.Error())
		return nil, err
	}
	return &WsServiceStream{conn: ws}, nil
}

type WsServiceStream struct {
	conn *websocket.Conn
}

func (c *WsServiceStream) Send(v interface{}) error {
	return c.conn.WriteJSON(v)
}

func (c *WsServiceStream) Recv(v interface{}) error {
	return c.conn.ReadJSON(v)
}

func (c *WsServiceStream) Close() error {
	return c.conn.Close()
}

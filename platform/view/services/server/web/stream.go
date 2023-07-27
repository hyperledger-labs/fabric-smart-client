/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

type SocketConn struct {
	ws     *websocket.Conn
	logger logger
}

func NewSocketConn(l logger, writer http.ResponseWriter, request *http.Request) (*SocketConn, error) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	ws, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return nil, err
	}
	l.Infof("Upgraded to web socket")
	return &SocketConn{ws: ws, logger: l}, nil
}

func (c *SocketConn) Recv(p any) error {
	message, err := c.Read()
	if err != nil {
		return err
	}
	return json.Unmarshal(message, p)
}

func (c *SocketConn) Send(p any) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return c.Write(data)
}

func (c *SocketConn) Read() ([]byte, error) {
	_, message, err := c.ws.ReadMessage()
	c.logger.Infof("Received message: %s", message)
	if err != nil {
		c.logger.Errorf("Error receiving message: %v", err)
	}
	return message, nil
}

func (c *SocketConn) Write(message []byte) error {
	c.logger.Infof("Sending message: %s", message)
	err := c.ws.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		c.logger.Errorf("Error writing message: %v", err)
	}
	return err
}

func (c *SocketConn) Close() error {
	c.logger.Infof("Closing web socket")
	return c.ws.Close()
}

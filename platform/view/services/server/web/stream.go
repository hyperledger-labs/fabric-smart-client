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

type Input struct {
	Raw []byte
}

type Output struct {
	Raw []byte
}

type WSStream struct {
	ws     *websocket.Conn
	logger logger
}

func OpenWSServerConn(writer http.ResponseWriter, request *http.Request) (*websocket.Conn, error) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
	return upgrader.Upgrade(writer, request, nil)
}

func NewWSStream(l logger, writer http.ResponseWriter, request *http.Request) (*WSStream, error) {
	ws, err := OpenWSServerConn(writer, request)
	if err != nil {
		return nil, err
	}
	l.Infof("Upgraded to web socket")
	return &WSStream{ws: ws, logger: l}, nil
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
	c.logger.Debugf("Received message: %s", message)
	if err != nil {
		c.logger.Errorf("Error receiving message: %v", err)
	}
	return message, nil
}

func (c *WSStream) Write(message []byte) error {
	c.logger.Debugf("Sending message: %s", message)
	err := c.ws.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		c.logger.Errorf("Error writing message: %v", err)
	}
	return err
}

func (c *WSStream) Close() error {
	c.logger.Debugf("Closing web socket")
	err := c.ws.Close()
	if err != nil {
		c.logger.Errorf("error closing web socket: %v", err)
	}
	return err
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

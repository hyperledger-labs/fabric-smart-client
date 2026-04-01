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

const maxMessageSize = 10 * 1024 * 1024

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
	logger.Debugf("received message: %s", message)
	if err != nil {
		logger.Errorf("error receiving message: %v", err)
	}
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

func (c *WSStream) Close() error {
	logger.Debugf("closing web socket")
	err := c.ws.Close()
	if err != nil {
		logger.Errorf("error closing web socket: %v", err)
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

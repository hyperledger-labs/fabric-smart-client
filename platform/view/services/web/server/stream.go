/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// maxMessageSize is the default read limit of a websocket message.
const maxMessageSize = 10 * 1024 * 1024

// OpenWSServerConn upgrades request to a websocket with the default read limit (10 MiB).
// Browsers may connect only from the server's own origin.
func OpenWSServerConn(writer http.ResponseWriter, request *http.Request) (*websocket.Conn, error) {
	return OpenWSServerConnWithLimit(writer, request, maxMessageSize)
}

// OpenWSServerConnWithLimit is OpenWSServerConn with an explicit read limit, in bytes, for a
// single websocket message. A non-positive limit means the default (10 MiB).
func OpenWSServerConnWithLimit(writer http.ResponseWriter, request *http.Request, readLimit int64) (*websocket.Conn, error) {
	if readLimit <= 0 {
		readLimit = maxMessageSize
	}
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
	conn.SetReadLimit(readLimit)
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

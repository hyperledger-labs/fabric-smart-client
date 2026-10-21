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
)

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
	return OpenWSClientConnContextWithLimit(ctx, url, config, maxMessageSize)
}

// OpenWSClientConnContextWithLimit is OpenWSClientConnContext with an explicit read limit, in
// bytes, for a single websocket message. A non-positive limit means the default (10 MiB).
func OpenWSClientConnContextWithLimit(ctx context.Context, url string, config *tls.Config, readLimit int64) (*websocket.Conn, error) { //nolint:contextcheck // nil ctx is treated as context.Background
	if readLimit <= 0 {
		readLimit = maxMessageSize
	}
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
	ws.SetReadLimit(readLimit)
	return ws, nil
}

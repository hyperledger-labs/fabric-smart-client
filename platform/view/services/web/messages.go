/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package web holds the JSON messages exchanged by the web server and the web client, so
// that the client does not depend on the server.
package web

// ResponseErr is the JSON body of the error responses the web server sends for a request
// to a registered URI.
type ResponseErr struct {
	// Reason describes the failure to the client.
	Reason string
}

// Input is a message sent by the client on a view's websocket stream.
type Input struct {
	Raw []byte
}

// Output is a message sent by the server on a view's websocket stream.
type Output struct {
	Raw []byte
}

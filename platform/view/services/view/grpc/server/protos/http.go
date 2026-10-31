/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protos

// The HTTP binding of ViewService. ProcessCommand is a POST whose body is a protojson
// SignedCommand and whose 200 response is a protojson SignedCommandResponse. StreamCommand is
// a WebSocket whose text frames carry the messages of the gRPC stream, protojson-encoded and
// in the same order.
const (
	ProcessCommandHTTPPath = "/v1/ViewService/ProcessCommand"
	StreamCommandHTTPPath  = "/v1/ViewService/StreamCommand"

	// DefaultHTTPMaxRecvMsgSize is the largest request body, response body or WebSocket
	// message an end of the HTTP binding reads when it is configured with no limit: 100 MiB.
	DefaultHTTPMaxRecvMsgSize = 100 * 1024 * 1024

	// AbortedByShutdownMessage is the message of the gRPC Aborted status a view call ends with
	// when the node's shutdown cancelled the view. A stream carries it as the reason of its
	// close frame, with code 1011, so that a client tells it from other server errors.
	AbortedByShutdownMessage = "view cancelled by node shutdown"
)

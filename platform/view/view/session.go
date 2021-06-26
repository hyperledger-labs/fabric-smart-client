/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"fmt"
)

const (
	OK    = 200
	ERROR = 500
)

type Message struct {
	SessionID    string // Session Identifier
	ContextID    string // Context Identifier
	Caller       string // View Identifier of the caller
	FromEndpoint string // Endpoint identifier of the caller
	FromPKID     []byte // PK identifier of the caller
	Status       int32  // Message Status (OK, ERROR)
	Payload      []byte // Payload
}

func (m *Message) String() string {
	return fmt.Sprintf("[session:%s,context:%s,caller:%s,endpoint:%s",
		m.SessionID,
		m.ContextID,
		m.Caller,
		m.FromEndpoint)
}

type SessionInfo struct {
	ID           string
	Caller       Identity
	CallerViewID string
	Endpoint     string
	EndpointPKID []byte
	Closed       bool
}

func (i *SessionInfo) String() string {
	return fmt.Sprintf("session info [%s,%s,%s,%s,%s,%v]", i.ID, i.Caller, i.Caller, i.Endpoint, i.EndpointPKID, i.Closed)
}

// Session encapsulates a communication channel to an endpoint
type Session interface {
	Info() SessionInfo

	// Send sends the payload to the endpoint
	Send(payload []byte) error

	// SendError sends an error to the endpoint with the passed payload
	SendError(payload []byte) error

	// Receive returns a channel of messages received from the endpoint
	Receive() <-chan *Message

	// Close releases all the resources allocated by this session
	Close()
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// SessionFactory is used to create new communication sessions.
//
//go:generate counterfeiter -o mock/session_factory.go -fake-name SessionFactory . SessionFactory
type SessionFactory interface {
	// NewSessionWithID returns a new session for the given arguments.
	NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg any) (view.Session, error)

	// NewSession returns a new session for the given arguments.
	NewSession(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error)

	// DeleteSessions deletes all sessions for the given session ID.
	DeleteSessions(ctx context.Context, sessionID string)
}

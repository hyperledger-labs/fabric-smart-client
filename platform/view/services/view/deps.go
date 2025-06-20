/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

//go:generate counterfeiter -o mock/comm_layer.go -fake-name CommLayer . CommLayer

type CommLayer interface {
	NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error)

	NewSession(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error)

	MasterSession() (view.Session, error)

	DeleteSessions(ctx context.Context, sessionID string)
}

func GetCommLayer(sp services.Provider) CommLayer {
	s, err := sp.GetService(reflect.TypeOf((*CommLayer)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(CommLayer)
}

//go:generate counterfeiter -o mock/session_factory.go -fake-name SessionFactory . SessionFactory

// SessionFactory is used to create new communication sessions
type SessionFactory interface {
	NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg *view.Message) (view.Session, error)

	NewSession(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error)

	DeleteSessions(ctx context.Context, sessionID string)
}

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

// CommLayer models the communication layer.
type CommLayer interface {
	// NewSessionWithID returns a new session for the given arguments.
	NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg interface{}) (view.Session, error)

	// NewSession returns a new session for the given arguments.
	NewSession(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error)

	// MasterSession returns the master session.
	MasterSession() (view.Session, error)

	// DeleteSessions deletes all sessions for the given session ID.
	DeleteSessions(ctx context.Context, sessionID string)
}

// GetCommLayer returns the communication layer from the service provider.
func GetCommLayer(sp services.Provider) CommLayer {
	s, err := sp.GetService(reflect.TypeOf((*CommLayer)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(CommLayer)
}

// GetEndpointService returns the endpoint service from the service provider.
func GetEndpointService(sp services.Provider) EndpointService {
	s, err := sp.GetService(reflect.TypeOf((*EndpointService)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(EndpointService)
}

// GetIdentityProvider returns the identity provider from the service provider.
func GetIdentityProvider(sp services.Provider) IdentityProvider {
	s, err := sp.GetService(reflect.TypeOf((*IdentityProvider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(IdentityProvider)
}

// GetLocalIdentityChecker returns the local identity checker from the service provider.
func GetLocalIdentityChecker(sp services.Provider) LocalIdentityChecker {
	s, err := sp.GetService(reflect.TypeOf((*LocalIdentityChecker)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(LocalIdentityChecker)
}

// SessionFactory is used to create new communication sessions.
//
//go:generate counterfeiter -o mock/session_factory.go -fake-name SessionFactory . SessionFactory
type SessionFactory interface {
	// NewSessionWithID returns a new session for the given arguments.
	NewSessionWithID(sessionID, contextID, endpoint string, pkid []byte, caller view.Identity, msg interface{}) (view.Session, error)

	// NewSession returns a new session for the given arguments.
	NewSession(caller string, contextID string, endpoint string, pkid []byte) (view.Session, error)

	// DeleteSessions deletes all sessions for the given session ID.
	DeleteSessions(ctx context.Context, sessionID string)
}

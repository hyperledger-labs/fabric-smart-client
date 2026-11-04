/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestComponent is the application component the version views negotiate. The initiator
// binary registers it at {2, 3} and the responder at {1, 2}, so their sessions use version 2.
const TestComponent = "pingtest"

// VersionInitiator opens a session to the responder and returns the version of TestComponent the session
// negotiated, after the responder confirmed it observes the same version.
type VersionInitiator struct{}

// Call implements view.View.
func (v *VersionInitiator) Call(viewCtx view.Context) (any, error) {
	identityProvider, err := id.GetProvider(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting identity provider")
	}
	s, err := viewCtx.GetSession(v, identityProvider.Identity("responder"))
	if err != nil {
		return nil, err
	}
	version, err := view.SessionVersion(viewCtx.Context(), s, TestComponent)
	if err != nil {
		return nil, err
	}
	if err := s.Send(viewCtx.Context(), []byte{byte(version)}); err != nil {
		return nil, err
	}
	reply, err := session.ReadMessageWithTimeout(s, roundTimeout)
	if err != nil {
		return nil, err
	}
	if len(reply) != 1 || uint32(reply[0]) != version {
		return nil, errors.Errorf("responder observes version %v, initiator %d", reply, version)
	}
	return version, nil
}

// VersionResponder answers a VersionInitiator with the version of TestComponent it observes.
type VersionResponder struct{}

// Call implements view.View.
func (*VersionResponder) Call(viewCtx view.Context) (any, error) {
	s := viewCtx.Session()
	if _, err := session.ReadMessageWithTimeout(s, roundTimeout); err != nil {
		return nil, err
	}
	version, err := view.SessionVersion(viewCtx.Context(), s, TestComponent)
	if err != nil {
		return nil, err
	}
	return nil, s.Send(viewCtx.Context(), []byte{byte(version)})
}

// VersionInitiatorViewFactory is the factory of VersionInitiator views.
type VersionInitiatorViewFactory struct{}

// NewView returns a new VersionInitiator.
func (*VersionInitiatorViewFactory) NewView([]byte) (view.View, error) {
	return &VersionInitiator{}, nil
}

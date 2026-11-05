/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Version names a party and a component whose negotiated format version to report.
type Version struct {
	// Party is the identity of the FSC node to open a session to.
	Party view.Identity
	// Component is the component, such as fabric, whose version to report.
	Component string
}

// VersionView opens a session to Party and returns the version of Component the session
// negotiated. It sends no message.
type VersionView struct {
	Version
}

func (v *VersionView) Call(viewCtx view.Context) (any, error) {
	s, err := viewCtx.GetSession(viewCtx.Initiator(), v.Party)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting session to [%s]", v.Party)
	}
	defer s.Close()
	return view.SessionVersion(viewCtx.Context(), s, v.Component)
}

type VersionViewFactory struct{}

func (*VersionViewFactory) NewView(in []byte) (view.View, error) {
	f := &VersionView{}
	if err := json.Unmarshal(in, &f.Version); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling input")
	}
	return f, nil
}

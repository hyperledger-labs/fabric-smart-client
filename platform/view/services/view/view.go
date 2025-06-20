/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// RunCall is a shortcut for `context.RunView(nil, view.WithViewCall(v))` and can be used to run a view call in a given context.
func RunCall(context view.Context, v func(context view.Context) (interface{}, error)) (interface{}, error) {
	return context.RunView(
		nil,
		view.WithViewCall(v),
	)
}

// Initiate initiates a new protocol whose initiator's view is the passed one.
// The execution happens in a freshly created context.
// This is a shortcut for `view.GetManager(context).InitiateView(initiator)`.
func Initiate(context view.Context, initiator view.View) (interface{}, error) {
	m, err := GetManager(context)
	if err != nil {
		return nil, err
	}
	return m.InitiateView(initiator, context.Context())
}

// AsResponder can be used by an initiator to behave temporarily as a responder.
// Recall that a responder is characterized by having a default session (`context.Session()`) established by an initiator.
func AsResponder(context view.Context, session view.Session, v func(context view.Context) (interface{}, error)) (interface{}, error) {
	return context.RunView(
		nil,
		view.WithViewCall(v),
		view.AsResponder(session),
	)
}

// AsInitiatorCall can be used by a responder to behave temporarily as an initiator.
// Recall that an initiator is characterized by having an initiator (`context.Initiator()`) set when the initiator is instantiated.
// AsInitiatorCall sets context.Initiator() to the passed initiator, and executes the passed view call.
// TODO: what happens to the sessions already openend with a different initiator (maybe an empty one)?
func AsInitiatorCall(context view.Context, initiator view.View, v func(context view.Context) (interface{}, error)) (interface{}, error) {
	return context.RunView(
		initiator,
		view.WithViewCall(v),
		view.AsInitiator(),
	)
}

// AsInitiatorView can be used by a responder to behave temporarily as an initiator.
// Recall that an initiator is characterized by having an initiator (`context.Initiator()`) set when the initiator is instantiated.
// AsInitiatorView sets context.Initiator() to the passed initiator, and executes it.
func AsInitiatorView(context view.Context, initiator view.View) (interface{}, error) {
	return context.RunView(
		initiator,
		view.AsInitiator(),
	)
}

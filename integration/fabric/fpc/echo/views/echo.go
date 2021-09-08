/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Echo models the parameters to be used to invoke the Echo FPC
type Echo struct {
	// Function to invoke
	Function string
	// Args to pass to the function
	Args []string
}

// EchoView models a View that invokes the Echo FPC
type EchoView struct {
	*Echo
}

func (e *EchoView) Call(context view.Context) (interface{}, error) {
	// Invoke the `echo` chaincode deployed on the default channel of the default Fbairc network
	res, err := fpc.GetDefaultChannel(context).Chaincode(
		"echo",
	).Invoke(
		e.Function, fpc.StringsToArgs(e.Args)...,
	).Call()
	assert.NoError(err, "failed invoking echo")
	assert.Equal(e.Function, string(res))

	return res, nil
}

type EchoViewFactory struct{}

func (l *EchoViewFactory) NewView(in []byte) (view.View, error) {
	f := &EchoView{}
	assert.NoError(json.Unmarshal(in, &f.Echo))
	return f, nil
}

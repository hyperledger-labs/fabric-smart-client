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

type Echo struct {
	Function string
	Args     []string
}

type EchoView struct {
	*Echo
}

func (e *EchoView) Call(context view.Context) (interface{}, error) {
	var passedArgs []interface{}
	for _, arg := range e.Args {
		passedArgs = append(passedArgs, arg)
	}

	res, err := fpc.GetDefaultChannel(context).Chaincode("echo").Invoke(
		e.Function, passedArgs...,
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

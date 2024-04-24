/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
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
	ch, err := fpc.GetDefaultChannel(context)
	assert.NoError(err)
	v, err := ch.EnclaveRegistry().IsAvailable()
	assert.NoError(err, "failed checking availability of the enclave registry")
	assert.True(v, "the enclave registry is not available")

	v, err = ch.EnclaveRegistry().IsPrivate("echo")
	assert.NoError(err, "failed checking echo deployment")
	assert.True(v, "echo should be an FPC")

	v, err = ch.EnclaveRegistry().IsPrivate("mycc")
	assert.NoError(err, "failed checking mycc deployment")
	assert.False(v, "mycc should be a standard CC")

	// Invoke the `echo` chaincode deployed on the default channel of the default Fabric network
	res, err := ch.Chaincode(
		"echo",
	).Invoke(
		e.Function, fpc.StringsToArgs(e.Args)...,
	).Call()
	assert.NoError(err, "failed invoking echo")
	assert.Equal(e.Function, string(res))

	fns, err := fabric.GetDefaultFNS(context)
	assert.NoError(err)
	_, res, err = chaincode.NewInvokeView(
		"echo",
		e.Function,
		fpc.StringsToArgs(e.Args)...,
	).WithSignerIdentity(
		fns.LocalMembership().DefaultIdentity(),
	).Invoke(context)
	assert.NoError(err, "failed invoking echo")
	assert.Equal(e.Function, string(res))

	// Query the `echo` chaincode deployed on the default channel of the default Fabric network
	res, err = ch.Chaincode(
		"echo",
	).Query(
		e.Function, fpc.StringsToArgs(e.Args)...,
	).WithSignerIdentity(
		fns.LocalMembership().DefaultIdentity(),
	).Call()
	assert.NoError(err, "failed querying echo")
	assert.Equal(e.Function, string(res))

	res, err = chaincode.NewQueryView(
		"echo",
		e.Function,
		fpc.StringsToArgs(e.Args)...,
	).WithSignerIdentity(
		fns.LocalMembership().DefaultIdentity(),
	).Query(context)
	assert.NoError(err, "failed querying echo")
	assert.Equal(e.Function, string(res))

	// Endorse the `echo` chaincode deployed on the default channel of the default Fabric network
	envelope, err := ch.Chaincode(
		"echo",
	).Endorse(
		e.Function, fpc.StringsToArgs(e.Args)...,
	).WithSignerIdentity(
		fns.LocalMembership().DefaultIdentity(),
	).Call()
	assert.NoError(err, "failed endorsing echo")
	assert.NotNil(envelope)
	assert.NoError(fns.Ordering().Broadcast(context.Context(), envelope))

	envelope, err = chaincode.NewEndorseView(
		"echo",
		e.Function,
		fpc.StringsToArgs(e.Args)...,
	).WithSignerIdentity(
		fns.LocalMembership().DefaultIdentity(),
	).WithNumRetries(4).WithRetrySleep(2 * time.Second).Endorse(context)
	assert.NoError(err, "failed endorsing echo")
	assert.Equal(e.Function, string(res))
	assert.NoError(fns.Ordering().Broadcast(context.Context(), envelope))

	return res, nil
}

type EchoViewFactory struct{}

func (l *EchoViewFactory) NewView(in []byte) (view.View, error) {
	f := &EchoView{}
	assert.NoError(json.Unmarshal(in, &f.Echo))
	return f, nil
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type InvokeCall struct {
	InvokerIdentity    view.Identity
	Network            string
	Channel            string
	ChaincodePath      string
	ChaincodeName      string
	ChaincodeVersion   string
	TransientMap       map[string]interface{}
	Endorsers          []view.Identity
	EndorsersMSPIDs    []string
	EndorsersFromMyOrg bool
	Function           string
	Args               []interface{}
}

type invokeChaincodeView struct {
	*InvokeCall
}

func NewInvokeView(chaincode, function string, args ...interface{}) *invokeChaincodeView {
	return &invokeChaincodeView{
		InvokeCall: &InvokeCall{
			ChaincodeName: chaincode,
			Function:      function,
			Args:          args,
		},
	}
}

func (i *invokeChaincodeView) Call(context view.Context) (interface{}, error) {
	txid, result, err := i.Invoke(context)
	if err != nil {
		return nil, err
	}
	return []interface{}{txid, result}, nil
}

func (i *invokeChaincodeView) Invoke(context view.Context) (string, []byte, error) {
	// TODO: endorse and then send to ordering
	if len(i.ChaincodeName) == 0 {
		return "", nil, errors.Errorf("no chaincode specified")
	}

	fNetwork := fabric.GetFabricNetworkService(context, i.Network)
	if fNetwork == nil {
		return "", nil, errors.Errorf("fabric network service [%s] not found", i.Network)
	}
	channel, err := fNetwork.Channel(i.Channel)
	if err != nil {
		return "", nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", i.Network, i.Channel)
	}
	if i.InvokerIdentity.IsNone() {
		i.InvokerIdentity = fNetwork.IdentityProvider().DefaultIdentity()
	}
	chaincode := channel.Chaincode(i.ChaincodeName)
	if chaincode == nil {
		return "", nil, errors.Errorf("fabric chaincode [%s:%s:%s] not found", i.Network, i.Channel, i.ChaincodeName)
	}
	if chaincode.IsPrivate() {
		// This is a Fabric Private Chaincode, use the corresponding service
		fpcChannel := fpc.GetChannel(context, i.Network, i.Channel)
		res, err := fpcChannel.Chaincode(i.ChaincodeName).Invoke(i.Function, i.Args...).Call()
		return "", res, err
	}

	invocation := chaincode.Invoke(i.Function, i.Args...).WithInvokerIdentity(i.InvokerIdentity)
	for k, v := range i.TransientMap {
		invocation.WithTransientEntry(k, v)
	}
	if len(i.Endorsers) != 0 {
		invocation.WithEndorsers(i.Endorsers...)
	}
	if len(i.EndorsersMSPIDs) != 0 {
		invocation.WithEndorsersByMSPIDs(i.EndorsersMSPIDs...)
	}
	if i.EndorsersFromMyOrg {
		invocation.WithEndorsersFromMyOrg()
	}

	txid, result, err := invocation.Submit()
	if err != nil {
		return "", nil, err
	}
	return txid, result, nil
}

func (i *invokeChaincodeView) WithTransientEntry(k string, v interface{}) *invokeChaincodeView {
	if i.TransientMap == nil {
		i.TransientMap = map[string]interface{}{}
	}
	i.TransientMap[k] = v
	return i
}

func (i *invokeChaincodeView) WithEndorsers(ids ...view.Identity) *invokeChaincodeView {
	i.InvokeCall.Endorsers = ids
	return i
}

func (i *invokeChaincodeView) WithNetwork(name string) *invokeChaincodeView {
	i.InvokeCall.Network = name
	return i
}

func (i *invokeChaincodeView) WithChannel(name string) *invokeChaincodeView {
	i.InvokeCall.Channel = name
	return i
}

func (i *invokeChaincodeView) WithEndorsersByMSPIDs(mspIDs ...string) *invokeChaincodeView {
	i.InvokeCall.EndorsersMSPIDs = mspIDs
	return i
}

func (i *invokeChaincodeView) WithEndorsersFromMyOrg() *invokeChaincodeView {
	i.EndorsersFromMyOrg = true
	return i
}

func (i *invokeChaincodeView) WithInvokerIdentity(id view.Identity) *invokeChaincodeView {
	i.InvokerIdentity = id
	return i
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Invoke struct {
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
	*Invoke
}

func NewInvokeView(chaincode, function string, args ...interface{}) *invokeChaincodeView {
	return &invokeChaincodeView{
		Invoke: &Invoke{
			ChaincodeName: chaincode,
			Function:      function,
			Args:          args,
		},
	}
}

func (i *invokeChaincodeView) Call(context view.Context) (interface{}, error) {
	if len(i.ChaincodeName) == 0 {
		return nil, errors.Errorf("no chaincode specified")
	}

	fNetwork := fabric.GetFabricNetworkService(context, i.Network)
	channel, err := fNetwork.Channel(i.Channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", i.Network, i.Channel)
	}
	if i.InvokerIdentity.IsNone() {
		i.InvokerIdentity = fNetwork.IdentityProvider().DefaultIdentity()
	}
	invocation := channel.Chaincode(i.ChaincodeName).Invoke(i.Function, i.Args...).WithSignerIdentity(i.InvokerIdentity)
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
		return nil, err
	}
	return []interface{}{txid, result}, nil
}

func (i *invokeChaincodeView) WithTransientEntry(k string, v interface{}) *invokeChaincodeView {
	if i.TransientMap == nil {
		i.TransientMap = map[string]interface{}{}
	}
	i.TransientMap[k] = v
	return i
}

func (i *invokeChaincodeView) WithEndorsers(ids ...view.Identity) *invokeChaincodeView {
	i.Invoke.Endorsers = ids
	return i
}

func (i *invokeChaincodeView) WithNetwork(name string) *invokeChaincodeView {
	i.Invoke.Network = name
	return i
}

func (i *invokeChaincodeView) WithChannel(name string) *invokeChaincodeView {
	i.Invoke.Channel = name
	return i
}

func (i *invokeChaincodeView) WithEndorsersByMSPIDs(mspIDs ...string) *invokeChaincodeView {
	i.Invoke.EndorsersMSPIDs = mspIDs
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

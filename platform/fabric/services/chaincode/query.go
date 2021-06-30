/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type queryChaincodeView struct {
	*Invoke
}

func NewQueryView(chaincode, function string, args ...interface{}) *queryChaincodeView {
	return &queryChaincodeView{
		Invoke: &Invoke{
			ChaincodeName: chaincode,
			Function:      function,
			Args:          args,
		},
	}
}

func (i *queryChaincodeView) Call(context view.Context) (interface{}, error) {
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
	invocation := channel.Chaincode(i.ChaincodeName).Query(i.Function, i.Args...).WithInvokerIdentity(i.InvokerIdentity)
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

	return invocation.Call()
}

func (i *queryChaincodeView) WithTransientEntry(k string, v interface{}) *queryChaincodeView {
	if i.TransientMap == nil {
		i.TransientMap = map[string]interface{}{}
	}
	i.TransientMap[k] = v
	return i
}

func (i *queryChaincodeView) WithEndorsers(ids ...view.Identity) *queryChaincodeView {
	i.Invoke.Endorsers = ids
	return i
}

func (i *queryChaincodeView) WithNetwork(name string) *queryChaincodeView {
	i.Invoke.Network = name
	return i
}

func (i *queryChaincodeView) WithChannel(name string) *queryChaincodeView {
	i.Invoke.Channel = name
	return i
}

func (i *queryChaincodeView) WithEndorsersFromMyOrg() *queryChaincodeView {
	i.EndorsersFromMyOrg = true
	return i
}

func (i *queryChaincodeView) WithInvokerIdentity(id view.Identity) *queryChaincodeView {
	i.InvokerIdentity = id
	return i
}

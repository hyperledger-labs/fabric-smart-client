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

type queryChaincodeView struct {
	*InvokeCall
}

func NewQueryView(chaincode, function string, args ...interface{}) *queryChaincodeView {
	return &queryChaincodeView{
		InvokeCall: &InvokeCall{
			ChaincodeName: chaincode,
			Function:      function,
			Args:          args,
		},
	}
}

func (i *queryChaincodeView) Call(context view.Context) (interface{}, error) {
	return i.Query(context)
}

func (i *queryChaincodeView) Query(context view.Context) ([]byte, error) {
	if len(i.ChaincodeName) == 0 {
		return nil, errors.Errorf("no chaincode specified")
	}

	fNetwork := fabric.GetFabricNetworkService(context, i.Network)
	if fNetwork == nil {
		return nil, errors.Errorf("fabric network service [%s] not found", i.Network)
	}
	channel, err := fNetwork.Channel(i.Channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", i.Network, i.Channel)
	}
	if i.InvokerIdentity.IsNone() {
		i.InvokerIdentity = fNetwork.IdentityProvider().DefaultIdentity()
	}

	var chaincode Chaincode = &stdChaincode{ch: channel.Chaincode(i.ChaincodeName)}
	if chaincode.IsPrivate() {
		logger.Debugf("chaincode [%s:%s:%s] is a FPC", i.Network, i.Channel, i.ChaincodeName)
		// This is a Fabric Private Chaincode, use the corresponding service
		fpcChannel := fpc.GetChannel(context, i.Network, i.Channel)
		chaincode = &fpcChaincode{ch: fpcChannel.Chaincode(i.ChaincodeName)}
	} else {
		logger.Debugf("chaincode [%s:%s:%s] is a standard chaincode", i.Network, i.Channel, i.ChaincodeName)
	}

	invocation := chaincode.Query(i.Function, i.Args...).WithInvokerIdentity(i.InvokerIdentity)
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
	i.InvokeCall.Endorsers = ids
	return i
}

func (i *queryChaincodeView) WithNetwork(name string) *queryChaincodeView {
	i.InvokeCall.Network = name
	return i
}

func (i *queryChaincodeView) WithChannel(name string) *queryChaincodeView {
	i.InvokeCall.Channel = name
	return i
}

func (i *queryChaincodeView) WithEndorsersFromMyOrg() *queryChaincodeView {
	i.EndorsersFromMyOrg = true
	return i
}

func (i *queryChaincodeView) WithSignerIdentity(id view.Identity) *queryChaincodeView {
	i.InvokerIdentity = id
	return i
}

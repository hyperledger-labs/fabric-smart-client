/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type endorseChaincodeView struct {
	*InvokeCall
}

func NewEndorseView(chaincode, function string, args ...interface{}) *endorseChaincodeView {
	return &endorseChaincodeView{
		InvokeCall: &InvokeCall{
			ChaincodeName: chaincode,
			Function:      function,
			Args:          args,
		},
	}
}

func (i *endorseChaincodeView) Call(context view.Context) (interface{}, error) {
	return i.Endorse(context)
}

func (i *endorseChaincodeView) Endorse(context view.Context) (*fabric.Envelope, error) {
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

	var chaincode Chaincode
	stdChannelChaincode := channel.Chaincode(i.ChaincodeName)
	if stdChannelChaincode.IsPrivate() {
		// This is a Fabric Private Chaincode, use the corresponding service
		fpcChannel := fpc.GetChannel(context, i.Network, i.Channel)
		chaincode = &fpcChaincode{fpcChannel.Chaincode(i.ChaincodeName)}
		logger.Debugf("chaincode [%s:%s:%s] is a FPC", i.Network, i.Channel, i.ChaincodeName)
	} else {
		chaincode = &stdChaincode{ch: stdChannelChaincode}
		logger.Debugf("chaincode [%s:%s:%s] is a standard chaincode", i.Network, i.Channel, i.ChaincodeName)
	}

	invocation := chaincode.Endorse(
		i.Function,
		i.Args...,
	).WithInvokerIdentity(
		i.InvokerIdentity,
	).WithTxID(
		*i.TxID,
	)
	for k, v := range i.TransientMap {
		invocation.WithTransientEntry(k, v)
	}
	if len(i.EndorsersMSPIDs) != 0 {
		invocation.WithEndorsersByMSPIDs(i.EndorsersMSPIDs...)
	}
	if i.EndorsersFromMyOrg {
		invocation.WithEndorsersFromMyOrg()
	}
	if i.SetNumRetries {
		invocation.WithNumRetries(i.NumRetries)
	}
	if i.SetRetrySleep {
		invocation.WithRetrySleep(i.RetrySleep)
	}

	envelope, err := invocation.Call()
	if err != nil {
		return nil, err
	}
	return envelope, nil
}

func (i *endorseChaincodeView) WithTransientEntry(k string, v interface{}) *endorseChaincodeView {
	if i.TransientMap == nil {
		i.TransientMap = map[string]interface{}{}
	}
	i.TransientMap[k] = v
	return i
}

func (i *endorseChaincodeView) WithNetwork(name string) *endorseChaincodeView {
	i.InvokeCall.Network = name
	return i
}

func (i *endorseChaincodeView) WithChannel(name string) *endorseChaincodeView {
	i.InvokeCall.Channel = name
	return i
}

func (i *endorseChaincodeView) WithEndorsersByMSPIDs(mspIDs ...string) *endorseChaincodeView {
	i.InvokeCall.EndorsersMSPIDs = mspIDs
	return i
}

func (i *endorseChaincodeView) WithEndorsersFromMyOrg() *endorseChaincodeView {
	i.EndorsersFromMyOrg = true
	return i
}

func (i *endorseChaincodeView) WithSignerIdentity(id view.Identity) *endorseChaincodeView {
	i.InvokeCall.InvokerIdentity = id
	return i
}

func (i *endorseChaincodeView) WithTxID(id fabric.TxID) *endorseChaincodeView {
	i.TxID = &fabric.TxID{
		Nonce:   id.Nonce,
		Creator: id.Creator,
	}
	return i
}

func (i *endorseChaincodeView) WithNumRetries(numRetries uint) *endorseChaincodeView {
	i.SetNumRetries = true
	i.NumRetries = numRetries
	return i
}

func (i *endorseChaincodeView) WithRetrySleep(duration time.Duration) *endorseChaincodeView {
	i.SetRetrySleep = true
	i.RetrySleep = duration
	return i
}

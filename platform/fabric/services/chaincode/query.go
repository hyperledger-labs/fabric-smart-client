/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
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

	fNetwork, err := fabric.GetFabricNetworkService(context, i.Network)
	if err != nil {
		return nil, err
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
	chaincode = &stdChaincode{ch: stdChannelChaincode}
	logger.Debugf("chaincode [%s:%s:%s] is a standard chaincode", i.Network, i.Channel, i.ChaincodeName)

	invocation := chaincode.Query(i.Function, i.Args...).WithInvokerIdentity(i.InvokerIdentity)
	for k, v := range i.TransientMap {
		invocation.WithTransientEntry(k, v)
	}
	if len(i.EndorsersMSPIDs) != 0 {
		invocation.WithEndorsersByMSPIDs(i.EndorsersMSPIDs...)
	}
	if i.EndorsersFromMyOrg {
		invocation.WithEndorsersFromMyOrg()
	}
	if i.MatchEndorsementPolicy {
		invocation.WithMatchEndorsementPolicy()
	}
	if i.SetNumRetries {
		invocation.WithNumRetries(i.NumRetries)
	}
	if i.SetRetrySleep {
		invocation.WithRetrySleep(i.RetrySleep)
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

func (i *queryChaincodeView) WithNetwork(name string) *queryChaincodeView {
	i.InvokeCall.Network = name
	return i
}

func (i *queryChaincodeView) WithChannel(name string) *queryChaincodeView {
	i.InvokeCall.Channel = name
	return i
}

func (i *queryChaincodeView) WithMatchEndorsementPolicy() *queryChaincodeView {
	i.InvokeCall.MatchEndorsementPolicy = true
	return i
}

func (i *queryChaincodeView) WithEndorsersByMSPIDs(mspIDs ...string) *queryChaincodeView {
	i.InvokeCall.EndorsersMSPIDs = mspIDs
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

func (i *queryChaincodeView) WithNumRetries(numRetries uint) *queryChaincodeView {
	i.SetNumRetries = true
	i.NumRetries = numRetries
	return i
}

func (i *queryChaincodeView) WithRetrySleep(duration time.Duration) *queryChaincodeView {
	i.SetRetrySleep = true
	i.RetrySleep = duration
	return i
}

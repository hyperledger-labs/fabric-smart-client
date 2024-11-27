/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type InvokeCall struct {
	InvokerIdentity        view.Identity
	Network                string
	Channel                string
	ChaincodePath          string
	ChaincodeName          string
	ChaincodeVersion       string
	TransientMap           map[string]interface{}
	EndorsersMSPIDs        []string
	EndorsersFromMyOrg     bool
	Function               string
	Args                   []interface{}
	MatchEndorsementPolicy bool
	SetNumRetries          bool
	NumRetries             uint
	SetRetrySleep          bool
	RetrySleep             time.Duration
	TxID                   fabric.TxID
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
	info := &info{
		chaincodeName: i.ChaincodeName,
		network:       i.Network,
		channel:       i.Channel,
		identitiy:     i.InvokerIdentity,
	}
	chaincode, err := getChaincode(context, info)

	if err != nil {
		return "", nil, err
	}
	i.InvokerIdentity = info.identitiy

	logger.Debugf("chaincode [%s:%s:%s] is a standard chaincode", i.Network, i.Channel, i.ChaincodeName)
	invocation := chaincode.Invoke(i.Function, i.Args...).WithInvokerIdentity(i.InvokerIdentity)
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

func (i *invokeChaincodeView) WithSignerIdentity(id view.Identity) *invokeChaincodeView {
	i.InvokerIdentity = id
	return i
}

func (i *invokeChaincodeView) WithNumRetries(numRetries uint) *invokeChaincodeView {
	i.SetNumRetries = true
	i.NumRetries = numRetries
	return i
}

func (i *invokeChaincodeView) WithRetrySleep(duration time.Duration) *invokeChaincodeView {
	i.SetRetrySleep = true
	i.RetrySleep = duration
	return i
}

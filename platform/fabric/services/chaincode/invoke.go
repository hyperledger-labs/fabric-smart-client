/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type InvokeCall struct {
	TxID                   *fabric.TxID
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

type InvokeChaincodeView struct {
	*InvokeCall
}

func NewInvokeView(chaincode, function string, args ...interface{}) *InvokeChaincodeView {
	return &InvokeChaincodeView{
		InvokeCall: &InvokeCall{
			ChaincodeName: chaincode,
			Function:      function,
			Args:          args,
		},
	}
}

func (i *InvokeChaincodeView) Call(context view.Context) (interface{}, error) {
	txID, result, err := i.Invoke(context)
	if err != nil {
		return nil, err
	}
	return []interface{}{txID, result}, nil
}

func (i *InvokeChaincodeView) Invoke(context view.Context) (string, []byte, error) {
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

	if chaincode.IsPrivate() {
		logger.Debugf("chaincode [%s:%s:%s] is a FPC", i.Network, i.Channel, i.ChaincodeName)
		// This is a Fabric Private Chaincode, use the corresponding service
		fpcChannel := fpc.GetChannel(context, i.Network, i.Channel)
		res, err := fpcChannel.Chaincode(i.ChaincodeName).Invoke(i.Function, i.Args...).Call()
		return "", res, err
	} else {
		logger.Debugf("chaincode [%s:%s:%s] is a standard chaincode", i.Network, i.Channel, i.ChaincodeName)
	}

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
	if i.TxID != nil {
		invocation.WithTxID(driver.TxID(*i.TxID))
	}

	txID, result, err := invocation.Submit()
	if err != nil {
		return "", nil, err
	}
	return txID, result, nil
}

func (i *InvokeChaincodeView) WithTransientEntry(k string, v interface{}) *InvokeChaincodeView {
	if i.TransientMap == nil {
		i.TransientMap = map[string]interface{}{}
	}
	i.TransientMap[k] = v
	return i
}

func (i *InvokeChaincodeView) WithNetwork(name string) *InvokeChaincodeView {
	i.InvokeCall.Network = name
	return i
}

func (i *InvokeChaincodeView) WithChannel(name string) *InvokeChaincodeView {
	i.InvokeCall.Channel = name
	return i
}

func (i *InvokeChaincodeView) WithEndorsersByMSPIDs(mspIDs ...string) *InvokeChaincodeView {
	i.InvokeCall.EndorsersMSPIDs = mspIDs
	return i
}

func (i *InvokeChaincodeView) WithEndorsersFromMyOrg() *InvokeChaincodeView {
	i.EndorsersFromMyOrg = true
	return i
}

func (i *InvokeChaincodeView) WithSignerIdentity(id view.Identity) *InvokeChaincodeView {
	i.InvokerIdentity = id
	return i
}

// WithTxID forces to use the passed transaction id
func (i *InvokeChaincodeView) WithTxID(id fabric.TxID) *InvokeChaincodeView {
	i.InvokeCall.TxID = &id
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

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
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
type RegisterChaincodeCall struct {
	InvokerIdentity  view.Identity
	Network          string
	Channel          string
	ChaincodePath    string
	ChaincodeName    string
	ChaincodeVersion string
	// Callback 		   ChaincodeEventCallback
}
type invokeChaincodeView struct {
	*InvokeCall
}

type registerChaincodeView struct {
	*RegisterChaincodeCall
}

type getInfo interface {
	getChannel()
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

//todo add options
func NewRegisterChaincodeView(chaincode string) *registerChaincodeView {
	return &registerChaincodeView{
		RegisterChaincodeCall: &RegisterChaincodeCall{
			ChaincodeName: chaincode,
			// Callback: callback,
		},
	}
}

type ChaincodeEventCallback func(event *committer.ChaincodeEvent) error

type info struct {
	chaincodeName string
	network       string
	channel       string
	identitiy     view.Identity
}

func (i *invokeChaincodeView) Call(context view.Context) (interface{}, error) {
	txid, result, err := i.Invoke(context)
	if err != nil {
		return nil, err
	}
	return []interface{}{txid, result}, nil
}

func (r *registerChaincodeView) Call(context view.Context) (interface{}, error) {
	events, err := r.RegisterChaincodeEvents(context)
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (i *invokeChaincodeView) Invoke(context view.Context) (string, []byte, error) {
	// TODO: endorse and then send to ordering
	chaincode, err := getChaincode(context, &info{
		chaincodeName: i.ChaincodeName,
		network:       i.Network,
		channel:       i.Channel,
		identitiy:     i.InvokerIdentity,
	})

	if err != nil {
		return "", nil, err
	}

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
	if len(i.Endorsers) != 0 {
		invocation.WithEndorsers(i.Endorsers...)
	}
	if len(i.EndorsersMSPIDs) != 0 {
		invocation.WithEndorsersByMSPIDs(i.EndorsersMSPIDs...)
	}
	if i.EndorsersFromMyOrg {
		invocation.WithEndorsersFromMyOrg()
	}
	fmt.Println("Submit chaincode")
	txid, result, err := invocation.Submit()
	if err != nil {
		return "", nil, err
	}
	return txid, result, nil
}

//add view to register chaincode
func (r *registerChaincodeView) RegisterChaincodeEvents(context view.Context) (<-chan *committer.ChaincodeEvent, error) {
	// TODO: endorse and then send to ordering
	chaincode, err := getChaincode(context, &info{
		chaincodeName: r.ChaincodeName,
		network:       r.Network,
		channel:       r.Channel,
		identitiy:     r.InvokerIdentity,
	})
	if err != nil {
		return nil, err
	}
	events, err := chaincode.EventListener.ChaincodeEvents()
	if err != nil {
		return nil, err
	}

	return events, nil
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

func (i *invokeChaincodeView) WithSignerIdentity(id view.Identity) *invokeChaincodeView {
	i.InvokerIdentity = id
	return i
}

func getChaincode(context view.Context, info *info) (*fabric.Chaincode, error) {
	if len(info.chaincodeName) == 0 {
		return nil, errors.Errorf("no chaincode specified")
	}

	fNetwork := fabric.GetFabricNetworkService(context, info.network)
	if fNetwork == nil {
		return nil, errors.Errorf("fabric network service [%s] not found", info.network)
	}
	channel, err := fNetwork.Channel(info.channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", info.network, info.channel)
	}
	if len(info.identitiy) == 0 {
		fNetwork.IdentityProvider().DefaultIdentity()
		info.identitiy = fNetwork.IdentityProvider().DefaultIdentity()
	}
	chaincode := channel.Chaincode(info.chaincodeName)
	if chaincode == nil {
		return nil, errors.Errorf("fabric chaincode %s not found", info.chaincodeName)
	}
	return chaincode, nil
}

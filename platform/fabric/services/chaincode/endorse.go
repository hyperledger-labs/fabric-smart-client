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

type EndorseCall struct {
	SignerIdentity     view.Identity
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

type endorseChaincodeView struct {
	*EndorseCall
}

func NewEndorseView(chaincode, function string, args ...interface{}) *endorseChaincodeView {
	return &endorseChaincodeView{
		EndorseCall: &EndorseCall{
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
	if i.SignerIdentity.IsNone() {
		i.SignerIdentity = fNetwork.IdentityProvider().DefaultIdentity()
	}
	chaincode := channel.Chaincode(i.ChaincodeName)
	if chaincode == nil {
		return nil, errors.Errorf("fabric chaincode [%s:%s:%s] not found", i.Network, i.Channel, i.ChaincodeName)
	}
	if chaincode.IsPrivate() {
		// This is a Fabric Private Chaincode, use the corresponding service
		fpcChannel := fpc.GetChannel(context, i.Network, i.Channel)
		return fpcChannel.Chaincode(i.ChaincodeName).Endorse(i.Function, i.Args...).Call()
	}

	invocation := chaincode.Endorse(i.Function, i.Args...).WithInvokerIdentity(i.SignerIdentity)
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

func (i *endorseChaincodeView) WithEndorsers(ids ...view.Identity) *endorseChaincodeView {
	i.EndorseCall.Endorsers = ids
	return i
}

func (i *endorseChaincodeView) WithNetwork(name string) *endorseChaincodeView {
	i.EndorseCall.Network = name
	return i
}

func (i *endorseChaincodeView) WithChannel(name string) *endorseChaincodeView {
	i.EndorseCall.Channel = name
	return i
}

func (i *endorseChaincodeView) WithEndorsersByMSPIDs(mspIDs ...string) *endorseChaincodeView {
	i.EndorseCall.EndorsersMSPIDs = mspIDs
	return i
}

func (i *endorseChaincodeView) WithEndorsersFromMyOrg() *endorseChaincodeView {
	i.EndorsersFromMyOrg = true
	return i
}

func (i *endorseChaincodeView) WithEndorserIdentity(id view.Identity) *endorseChaincodeView {
	i.SignerIdentity = id
	return i
}

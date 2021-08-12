/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger-labs/weaver-dlt-interoperability/sdks/fabric/go-sdk/interoperablehelper"
	"github.com/hyperledger-labs/weaver-dlt-interoperability/sdks/fabric/go-sdk/types"
	"github.com/hyperledger/fabric-protos-go/msp"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

func InteropFromContext(context view.Context, namespace string) {
	ns := fabric.GetDefaultFNS(context)
	names := fabric.GetFabricNetworkNames(ns.SP)

	if len(names) != 2 {
		panic(fmt.Sprintf("expected 2 network names but got %d", len(names)))
	}

	var otherName string
	for _, name := range names {
		if ns.Name() == name {
			continue
		}
		otherName = name
		break
	}

	otherNS := fabric.GetFabricNetworkService(ns.SP, otherName)

	//address := ns.ConfigService().GetString("weaver.relay.address")
	otherAddress := otherNS.ConfigService().GetString("weaver.relay.address")
	contract := FlowContract{
		context:   context,
		namespace: namespace,
	}

	invokeObject := types.Query{
		ContractName: namespace,
		Channel:      ns.Channels()[0],
		CcFunc:       "query",
		CcArgs:       []string{"alice"},
	}


	specialAddress := createAddress(invokeObject, otherNS.Name(), otherAddress)
	fmt.Println(">>>>", specialAddress)

	interopJSON := types.InteropJSON{
		Address:        specialAddress,
		ChaincodeFunc:  "query",
		ChaincodeId:    namespace,
		ChannelId:      ns.Channels()[0],
		RemoteEndPoint: otherAddress,
		NetworkId:      otherNS.Name(),
		Sign:           true,
		CcArgs:         []string{"alice"},
	}

	sIDBytes, err := ns.LocalMembership().DefaultSigningIdentity().Serialize()
	assert.NoError(err, "failed serializing signing identity")

	sID := &msp.SerializedIdentity{}
	err = proto.Unmarshal(sIDBytes, sID)
	assert.NoError(err, "failed unmarshaling serialized identity")

	me := fabric.GetDefaultIdentityProvider(context).DefaultIdentity()

	sigSvc := ns.SigService()
	signer, err := sigSvc.GetSigner(me)
	assert.NoError(err, "failed acquiring signer")

	_, _, err = interoperablehelper.InteropFlow(contract, ns.Name(), invokeObject, sID.Mspid, specialAddress, []int{1}, []types.InteropJSON{interopJSON}, signer, string(sID.IdBytes), true)
	assert.NoError(err, "failed running interop view")

}

func createAddress(query types.Query, networkId, remoteURL string) string {
	addressString := remoteURL + "/" + networkId + "/" + query.Channel + ":" + query.ContractName + ":" + query.CcFunc + ":" + query.CcArgs[0]
	return addressString
}

type FlowContract struct {
	context   view.Context
	namespace string
}

func (f FlowContract) transact(functionName string, args ...string) ([]byte, error) {
	context := f.context
	tx, err := state.NewTransaction(context)
	assert.NoError(err, "failed creating transaction")

	tx.SetNamespace(f.namespace)

	tx.AppendParameter([]byte(functionName))
	for _, arg := range args {
		tx.AppendParameter([]byte(arg))
	}

	res, err := context.RunView(state.NewCollectEndorsementsView(tx))
	assert.NoError(err, "failed collecting endorsement")

	resp := res.(*endorser.Transaction)
	return resp.Results()
}

func (f FlowContract) EvaluateTransaction(name string, args ...string) ([]byte, error) {
	return f.transact(name, args...)
}

func (f FlowContract) SubmitTransaction(name string, args ...string) ([]byte, error) {
	panic("we shouldn't use this")
}

type provider struct {
	mutex  sync.Mutex
	relays map[string]*Relay
}

func NewProvider() *provider {
	return &provider{relays: make(map[string]*Relay)}
}

func (w *provider) Relay(fns *fabric.NetworkService) *Relay {
	if fns == nil {
		panic("expected a fabric network service, got nil")
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	r, ok := w.relays[fns.Name()]
	if !ok {
		r = &Relay{
			fns: fns,
		}
		w.relays[fns.Name()] = r
	}
	return r
}

func GetProvider(sp view2.ServiceProvider) *provider {
	s, err := sp.GetService(reflect.TypeOf((*provider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(*provider)
}

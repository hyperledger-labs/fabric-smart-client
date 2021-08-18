/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package replacer

import (
	"fmt"
	"net"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/weaver-dlt-interoperability/sdks/fabric/go-sdk/interoperablehelper"
	"github.com/hyperledger-labs/weaver-dlt-interoperability/sdks/fabric/go-sdk/types"
	"github.com/hyperledger/fabric-protos-go/msp"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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

	address := ns.ConfigService().GetString("weaver.relay.address")
	otherAddress := otherNS.ConfigService().GetString("weaver.relay.address")
	strings.Replace(otherAddress, "127.0.0.1", fmt.Sprintf("relay-%s", otherNS.Name()), -1)

	fmt.Println(">>>> address:", address)

	_, port, err := net.SplitHostPort(address)
	assert.NoError(err, "failed splitting host and port")
	address = net.JoinHostPort("127.0.0.1", port)
	fmt.Println("changing address to", address)

	contract := FlowContract{
		context:   context,
		namespace: "interop",
	}

	invokeObject := types.Query{
		ContractName: namespace,
		Channel:      ns.Channels()[0],
		CcFunc:       "query",
		CcArgs:       []string{"alice"},
	}

	specialAddress := createAddress(invokeObject, otherNS.Name(), otherAddress)
	fmt.Println(">>>> specialAddress: ", specialAddress)

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

	_, _, err = interoperablehelper.InteropFlow(contract, ns.Name(), invokeObject, sID.Mspid, address, []int{1}, []types.InteropJSON{interopJSON}, signer, string(sID.IdBytes), true)
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

	var chaincodeArgs []interface{}
	for _, arg := range args {
		chaincodeArgs = append(chaincodeArgs, arg)
	}

	res, err := context.RunView(chaincode.NewQueryView(
		f.namespace,
		functionName,
		chaincodeArgs...,
	))
	assert.NoError(err, "failed invoking chaincode")

	return res.([]byte), nil
}

func (f FlowContract) EvaluateTransaction(name string, args ...string) ([]byte, error) {
	return f.transact(name, args...)
}

func (f FlowContract) SubmitTransaction(name string, args ...string) ([]byte, error) {
	panic("we shouldn't use this")
}

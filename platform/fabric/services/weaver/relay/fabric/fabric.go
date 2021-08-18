/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

type Fabric struct {
	fns *fabric.NetworkService
}

func New(fns *fabric.NetworkService) *Fabric {
	return &Fabric{fns: fns}
}

func (f *Fabric) Query(destination, function string, args ...interface{}) (*Query, error) {
	id, err := URLToID(destination)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing destination [%s]", destination)
	}
	return NewQuery(f.fns, id, function, args), nil
}

func (f *Fabric) VerifyProof(raw []byte) error {
	proof := &Proof{}
	if err := json.Unmarshal(raw, proof); err != nil {
		return errors.Wrapf(err, "failed unmarshalling proof")
	}

	channelName := f.fns.ConfigService().GetString("weaver.interopcc.channel")
	namespace := f.fns.ConfigService().GetString("weaver.interopcc.name")

	channel, err := f.fns.Channel(channelName)
	if err != nil {
		return errors.WithMessagef(err, "failed getting channel [%s:%s]", f.fns.Name(), channelName)
	}
	_, err = channel.Chaincode(namespace).Query(
		"VerifyView", proof.B64ViewProto, proof.Address,
	).WithInvokerIdentity(
		f.fns.IdentityProvider().DefaultIdentity(),
	).Call()
	if err != nil {
		return errors.WithMessagef(err, "failed invoking interop chaincode [%s.%s.%s:%s]", f.fns.Name(), channelName, namespace, "VerifyView")
	}
	return nil
}

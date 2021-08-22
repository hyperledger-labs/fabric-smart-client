/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

// Fabric models the relay services towards a Fabric network
type Fabric struct {
	fns *fabric.NetworkService
	ch  *fabric.Channel
}

func New(fns *fabric.NetworkService, ch *fabric.Channel) *Fabric {
	return &Fabric{fns: fns, ch: ch}
}

// Query invokes the passed function on the passed arguments on the passed destination network, and returns
// the result.
// The destination argument is expected to be formatted as 'fabric://<network-id>.<channel-id>.<chaincode-id>/`
func (f *Fabric) Query(destination, function string, args ...interface{}) (*Query, error) {
	id, err := URLToID(destination)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing destination [%s]", destination)
	}
	return NewQuery(f.fns, f.ch, id, function, args), nil
}

// VerifyProof checks the validity of the passed proof
func (f *Fabric) VerifyProof(raw []byte) error {
	proof := &Proof{}
	if err := json.Unmarshal(raw, proof); err != nil {
		return errors.Wrapf(err, "failed unmarshalling proof")
	}

	interopCCKey := fmt.Sprintf("weaver.interopcc.%s.name", f.ch.Name())
	namespace := f.fns.ConfigService().GetString(interopCCKey)
	logger.Debugf("verify proof at [%s:%s:%s:%s]", f.fns.Name(), f.ch.Name(), namespace, interopCCKey)
	_, err := f.ch.Chaincode(namespace).Query(
		"VerifyView", proof.B64ViewProto, proof.Address,
	).WithInvokerIdentity(
		f.fns.IdentityProvider().DefaultIdentity(),
	).Call()
	if err != nil {
		return errors.WithMessagef(err, "failed invoking interop chaincode [%s.%s.%s:%s]", f.fns.Name(), f.ch.Name(), namespace, "VerifyView")
	}
	return nil
}

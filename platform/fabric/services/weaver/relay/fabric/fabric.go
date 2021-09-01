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

// ProofFromBytes returns an object modeling a Relay proof from the passed bytes
func (f *Fabric) ProofFromBytes(raw []byte) (*Proof, error) {
	proof := &ProofMessage{}
	if err := json.Unmarshal(raw, proof); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling proof")
	}

	return NewProof(f.fns, f.ch, proof)
}

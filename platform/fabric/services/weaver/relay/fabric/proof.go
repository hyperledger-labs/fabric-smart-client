/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"encoding/base64"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/weaver-dlt-interoperability/common/protos-go/common"
	fabric2 "github.com/hyperledger-labs/weaver-dlt-interoperability/common/protos-go/fabric"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

// ProofMessage models a Relay proof for a Fabric Query
type ProofMessage struct {
	B64ViewProto string
	Address      string
}

// Proof models a Relay Proof generated from a Fabric a network
type Proof struct {
	fns    *fabric.NetworkService
	ch     *fabric.Channel
	proof  *ProofMessage
	view   *common.View
	fv     *fabric2.FabricView
	rwset  []byte
	result []byte
}

func NewProof(fns *fabric.NetworkService, ch *fabric.Channel, proof *ProofMessage) (*Proof, error) {
	viewRaw, err := base64.StdEncoding.DecodeString(proof.B64ViewProto)
	if err != nil {
		return nil, errors.Wrapf(err, "failed decoding view proto [%s]", proof.B64ViewProto)
	}

	view := &common.View{}
	if err := proto.Unmarshal(viewRaw, view); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling view")
	}
	if view.Meta.Protocol != common.Meta_FABRIC {
		return nil, errors.Errorf("invalid protocol, expected Meta_FABRIC, got [%d]", view.Meta.Protocol)
	}
	fv := &fabric2.FabricView{}
	if err := proto.Unmarshal(view.Data, fv); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling view's data")
	}

	respPayload, err := protoutil.UnmarshalChaincodeAction(fv.ProposalResponsePayload.Extension)
	if err != nil {
		err = errors.Errorf("GetChaincodeAction error %s", err)
		return nil, err
	}

	interopPayload := &common.InteropPayload{}
	err = proto.Unmarshal(fv.Response.Payload, interopPayload)
	if err != nil {
		return nil, errors.Errorf("failed to unmarshal InteropPayload: %v", err)
	}

	return &Proof{
		fns:    fns,
		ch:     ch,
		proof:  proof,
		view:   view,
		fv:     fv,
		rwset:  respPayload.Results,
		result: interopPayload.Payload,
	}, nil
}

// Verify checks the validity of this proof
func (p *Proof) Verify() error {
	interopCCKey := fmt.Sprintf("weaver.interopcc.%s.name", p.ch.Name())
	namespace := p.fns.ConfigService().GetString(interopCCKey)
	logger.Debugf("verify proof at [%s:%s:%s:%s]", p.fns.Name(), p.ch.Name(), namespace, interopCCKey)
	_, err := p.ch.Chaincode(namespace).Query(
		"VerifyView", p.proof.B64ViewProto, p.proof.Address,
	).WithInvokerIdentity(
		p.fns.IdentityProvider().DefaultIdentity(),
	).Call()
	if err != nil {
		return errors.WithMessagef(err, "failed invoking interop chaincode [%s.%s.%s:%s]", p.fns.Name(), p.ch.Name(), namespace, "VerifyView")
	}
	return nil
}

// IsOK return true if the result is valid
func (p *Proof) IsOK() bool {
	return p.fv.Response.Status == OK
}

// Result returns the response payload
func (p *Proof) Result() []byte {
	return p.result
}

// RWSet returns a wrapper over the Fabric rwset to inspect it
func (p *Proof) RWSet() (*Inspector, error) {
	i := newInspector()
	i.raw = p.rwset
	if err := i.rws.populate(p.rwset, "ephemeral"); err != nil {
		return nil, err
	}
	return i, nil
}

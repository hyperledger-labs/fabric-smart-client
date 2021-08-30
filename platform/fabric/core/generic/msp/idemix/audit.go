/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"encoding/json"

	csp "github.com/IBM/idemix/bccsp/schemes"
	idemix "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	"github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"
)

type AuditInfo struct {
	*csp.NymEIDAuditData
	Attributes [][]byte
	IPK        *idemix.IssuerPublicKey
}

func (a *AuditInfo) Bytes() ([]byte, error) {
	return json.Marshal(a)
}

func (a *AuditInfo) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, a)
}

func (a *AuditInfo) EnrollmentID() string {
	return string(a.Attributes[2])
}

func (a *AuditInfo) Match(id []byte) error {
	si := &m.SerializedIdentity{}
	err := proto.Unmarshal(id, si)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal to msp.SerializedIdentity{}")
	}

	serialized := new(m.SerializedIdemixIdentity)
	err = proto.Unmarshal(si.IdBytes, serialized)
	if err != nil {
		return errors.Wrap(err, "could not deserialize a SerializedIdemixIdentity")
	}

	sig := &idemix.Signature{}
	if err := proto.Unmarshal(serialized.Proof, sig); err != nil {
		return err
	}

	curve := math.Curves[math.FP256BN_AMCL]
	tr := &amcl.Fp256bn{C: curve}

	H_a_eid, err := tr.G1FromProto(a.IPK.HAttrs[eidIndex])
	if err != nil {
		return errors.Wrap(err, "could not deserialize HAttrs")
	}

	HRand, err := tr.G1FromProto(a.IPK.HRand)
	if err != nil {
		return errors.Wrap(err, "could not deserialize HAttrs")
	}

	EidNym, err := tr.G1FromProto(sig.EidNym.Nym)
	if err != nil {
		return errors.Wrap(err, "could not deserialize HAttrs")
	}

	eidAttr := curve.HashToZr(a.Attributes[eidIndex])
	Nym_eid := H_a_eid.Mul2(eidAttr, HRand, a.NymEIDAuditData.RNymEid)

	if !Nym_eid.Equals(EidNym) {
		return errors.New("eid nym does not match")
	}

	return nil
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package crypto

import (
	"bytes"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

func VerifyAuditingInfo(signature []byte, info *csp.IdemixSignatureInfo) (*Signature, error) {
	sig := &Signature{}
	if err := proto.Unmarshal(signature, sig); err != nil {
		return nil, err
	}

	proofC := FP256BN.FromBytes(sig.ProofC)
	for i, j := range hiddenIndices(info.Disclosure) {
		var rAttr = FP256BN.FromBytes(info.RAttrs[i])
		var attr = FP256BN.FromBytes(info.Attrs[j])
		c := BigToBytes(
			// s_attrsi = rAttrsi + C \cdot cred.Attrs[j]
			Modadd(rAttr, FP256BN.Modmul(proofC, attr, GroupOrder), GroupOrder),
		)
		if !bytes.Equal(sig.ProofSAttrs[i], c) {
			return nil, errors.Errorf("attribute mistmatch at [%d]", i)
		}
	}

	return sig, nil
}

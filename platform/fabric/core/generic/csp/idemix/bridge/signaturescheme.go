/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package bridge

import (
	"crypto/ecdsa"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp/idemix/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp/idemix/handlers"
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/hyperledger/fabric-amcl/amcl/FP256BN"
	"github.com/pkg/errors"
)

// SignatureScheme encapsulates the idemix algorithms to sign and verify using an idemix credential.
type SignatureScheme struct {
	NewRand func() *amcl.RAND
}

// Sign produces an idemix-signature with the respect to the passed serialised credential (cred),
// user secret key (sk), pseudonym public key (Nym) and secret key (RNym), issuer public key (ipk),
// and attributes to be disclosed.
func (s *SignatureScheme) Sign(cred []byte, sk handlers.Big, Nym handlers.Ecp, RNym handlers.Big, ipk handlers.IssuerPublicKey, attributes []csp.IdemixAttribute, msg []byte, rhIndex int, criRaw []byte) (res []byte, info *csp.IdemixSignatureInfo, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	isk, ok := sk.(*Big)
	if !ok {
		return nil, nil, errors.Errorf("invalid user secret key, expected *Big, got [%T]", sk)
	}
	inym, ok := Nym.(*Ecp)
	if !ok {
		return nil, nil, errors.Errorf("invalid nym public key, expected *Ecp, got [%T]", Nym)
	}
	irnym, ok := RNym.(*Big)
	if !ok {
		return nil, nil, errors.Errorf("invalid nym secret key, expected *Big, got [%T]", RNym)
	}
	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return nil, nil, errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	credential := &crypto.Credential{}
	err = proto.Unmarshal(cred, credential)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed unmarshalling credential")
	}

	cri := &crypto.CredentialRevocationInformation{}
	err = proto.Unmarshal(criRaw, cri)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed unmarshalling credential revocation information")
	}

	disclosure := make([]byte, len(attributes))
	for i := 0; i < len(attributes); i++ {
		if attributes[i].Type == csp.IdemixHiddenAttribute {
			disclosure[i] = 0
		} else {
			disclosure[i] = 1
		}
	}

	sig, sigInfo, err := crypto.NewSignature(
		credential,
		isk.E,
		inym.E,
		irnym.E,
		iipk.PK,
		disclosure,
		msg,
		rhIndex,
		cri,
		s.NewRand())
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed creating new signature")
	}

	sigRaw, err := proto.Marshal(sig)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed marshalling signature")
	}

	return sigRaw, sigInfo, nil
}

// Verify checks that an idemix signature is valid with the respect to the passed issuer public key, digest, attributes,
// revocation index (rhIndex), revocation public key, and epoch.
func (*SignatureScheme) Verify(ipk handlers.IssuerPublicKey, signature, digest []byte, attributes []csp.IdemixAttribute, rhIndex int, revocationPublicKey *ecdsa.PublicKey, epoch int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	iipk, ok := ipk.(*IssuerPublicKey)
	if !ok {
		return errors.Errorf("invalid issuer public key, expected *IssuerPublicKey, got [%T]", ipk)
	}

	sig := &crypto.Signature{}
	err = proto.Unmarshal(signature, sig)
	if err != nil {
		return err
	}

	disclosure := make([]byte, len(attributes))
	attrValues := make([]*FP256BN.BIG, len(attributes))
	for i := 0; i < len(attributes); i++ {
		switch attributes[i].Type {
		case csp.IdemixHiddenAttribute:
			disclosure[i] = 0
			attrValues[i] = nil
		case csp.IdemixBytesAttribute:
			disclosure[i] = 1
			attrValues[i] = crypto.HashModOrder(attributes[i].Value.([]byte))
		case csp.IdemixIntAttribute:
			disclosure[i] = 1
			attrValues[i] = FP256BN.NewBIGint(attributes[i].Value.(int))
		default:
			err = errors.Errorf("attribute type not allowed or supported [%v] at position [%d]", attributes[i].Type, i)
		}
	}
	if err != nil {
		return
	}

	return sig.Ver(
		disclosure,
		iipk.PK,
		digest,
		attrValues,
		rhIndex,
		revocationPublicKey,
		epoch)
}

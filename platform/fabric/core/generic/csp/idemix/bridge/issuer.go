/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package bridge

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp/idemix/crypto"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-amcl/amcl"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp/idemix/handlers"
)

// IssuerPublicKey encapsulate an idemix issuer public key.
type IssuerPublicKey struct {
	PK *crypto.IssuerPublicKey
}

func (o *IssuerPublicKey) Bytes() ([]byte, error) {
	return proto.Marshal(o.PK)
}

func (o *IssuerPublicKey) Hash() []byte {
	return o.PK.Hash
}

// IssuerSecretKey encapsulate an idemix issuer secret key.
type IssuerSecretKey struct {
	SK *crypto.IssuerKey
}

func (o *IssuerSecretKey) Bytes() ([]byte, error) {
	return proto.Marshal(o.SK)
}

func (o *IssuerSecretKey) Public() handlers.IssuerPublicKey {
	return &IssuerPublicKey{o.SK.Ipk}
}

// Issuer encapsulates the idemix algorithms to generate issuer key-pairs
type Issuer struct {
	NewRand func() *amcl.RAND
}

// NewKey generates a new issuer key-pair
func (i *Issuer) NewKey(attributeNames []string) (res handlers.IssuerSecretKey, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	sk, err := crypto.NewIssuerKey(attributeNames, i.NewRand())
	if err != nil {
		return
	}

	res = &IssuerSecretKey{SK: sk}

	return
}

func (*Issuer) NewPublicKeyFromBytes(raw []byte, attributes []string) (res handlers.IssuerPublicKey, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = nil
			err = errors.Errorf("failure [%s]", r)
		}
	}()

	ipk := new(crypto.IssuerPublicKey)
	err = proto.Unmarshal(raw, ipk)
	if err != nil {
		return nil, errors.WithStack(&csp.IdemixIssuerPublicKeyImporterError{
			Type:     csp.IdemixIssuerPublicKeyImporterUnmarshallingError,
			ErrorMsg: "failed to unmarshal issuer public key",
			Cause:    err})
	}

	err = ipk.SetHash()
	if err != nil {
		return nil, errors.WithStack(&csp.IdemixIssuerPublicKeyImporterError{
			Type:     csp.IdemixIssuerPublicKeyImporterHashError,
			ErrorMsg: "setting the hash of the issuer public key failed",
			Cause:    err})
	}

	err = ipk.Check()
	if err != nil {
		return nil, errors.WithStack(&csp.IdemixIssuerPublicKeyImporterError{
			Type:     csp.IdemixIssuerPublicKeyImporterValidationError,
			ErrorMsg: "invalid issuer public key",
			Cause:    err})
	}

	if len(attributes) != 0 {
		// Check the attributes
		if len(attributes) != len(ipk.AttributeNames) {
			return nil, errors.WithStack(&csp.IdemixIssuerPublicKeyImporterError{
				Type: csp.IdemixIssuerPublicKeyImporterNumAttributesError,
				ErrorMsg: fmt.Sprintf("invalid number of attributes, expected [%d], got [%d]",
					len(ipk.AttributeNames), len(attributes)),
			})
		}

		for i, attr := range attributes {
			if ipk.AttributeNames[i] != attr {
				return nil, errors.WithStack(&csp.IdemixIssuerPublicKeyImporterError{
					Type:     csp.IdemixIssuerPublicKeyImporterAttributeNameError,
					ErrorMsg: fmt.Sprintf("invalid attribute name at position [%d]", i),
				})
			}
		}
	}

	res = &IssuerPublicKey{PK: ipk}

	return
}

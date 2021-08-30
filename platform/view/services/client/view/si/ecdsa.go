/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package si

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"math/big"
)

var (
	// curveHalfOrders contains the precomputed curve group orders halved.
	// It is used to ensure that signature' S value is lower or equal to the
	// curve group order halved. We accept only low-S signatures.
	// They are precomputed for efficiency reasons.
	curveHalfOrders = map[elliptic.Curve]*big.Int{
		elliptic.P224(): new(big.Int).Rsh(elliptic.P224().Params().N, 1),
		elliptic.P256(): new(big.Int).Rsh(elliptic.P256().Params().N, 1),
		elliptic.P384(): new(big.Int).Rsh(elliptic.P384().Params().N, 1),
		elliptic.P521(): new(big.Int).Rsh(elliptic.P521().Params().N, 1),
	}
)

type ecdsaSignature struct {
	R, S *big.Int
}

type ecdsaSigner struct {
	sk *ecdsa.PrivateKey
}

func NewEcdsaSigner(sk *ecdsa.PrivateKey) *ecdsaSigner {
	return &ecdsaSigner{sk: sk}
}

func (d *ecdsaSigner) Sign(message []byte) ([]byte, error) {
	dgst := sha256.Sum256(message)

	r, s, err := ecdsa.Sign(rand.Reader, d.sk, dgst[:])
	if err != nil {
		return nil, err
	}

	s, _, err = toLowS(&d.sk.PublicKey, s)
	if err != nil {
		return nil, err
	}

	return asn1.Marshal(ecdsaSignature{R: r, S: s})
}

func isLowS(k *ecdsa.PublicKey, s *big.Int) (bool, error) {
	halfOrder, ok := curveHalfOrders[k.Curve]
	if !ok {
		return false, fmt.Errorf("curve not recognized [%s]", k.Curve)
	}

	return s.Cmp(halfOrder) != 1, nil

}

func toLowS(k *ecdsa.PublicKey, s *big.Int) (*big.Int, bool, error) {
	lowS, err := isLowS(k, s)
	if err != nil {
		return nil, false, err
	}

	if !lowS {
		// Set s to N - s that will be then in the lower part of signature space
		// less or equal to half order
		s.Sub(k.Params().N, s)

		return s, true, nil
	}

	return s, false, nil
}

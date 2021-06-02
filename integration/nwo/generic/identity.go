/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"
)

type Identity interface {
	Serialize() ([]byte, error)
}

type SigningIdentity interface {
	Identity
	Sign(msg []byte) ([]byte, error)
}

func (p *platform) GetSigningIdentity(peer *Peer) (SigningIdentity, error) {
	cert, err := ioutil.ReadFile(p.PeerLocalMSPIdentityCert(peer))
	if err != nil {
		return nil, err
	}
	sk, err := ioutil.ReadFile(p.PeerLocalMSPPrivateKey(peer))
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(sk)
	if block == nil {
		return nil, fmt.Errorf("failed decoding PEM. Block must be different from nil. [% x]", sk)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return &signingIdentity{
		serialized: cert,
		privateKey: k.(*ecdsa.PrivateKey),
	}, nil
}

type signingIdentity struct {
	serialized []byte
	privateKey *ecdsa.PrivateKey
}

func (s *signingIdentity) Serialize() ([]byte, error) {
	return s.serialized, nil
}

func (s *signingIdentity) Sign(message []byte) ([]byte, error) {
	hash := sha256.New()
	n, err := hash.Write(message)
	if n != len(message) {
		return nil, errors.Errorf("hash failure")
	}
	if err != nil {
		return nil, err
	}
	digest := hash.Sum(nil)

	return s.privateKey.Sign(rand.Reader, digest, nil)
}

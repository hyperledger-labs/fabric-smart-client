/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

type Signer interface {
	Sign(message []byte) ([]byte, error)
}

type SerializableSigner interface {
	Sign(message []byte) ([]byte, error)
	Serialize() ([]byte, error)
}

type signerWrapper struct {
	creator view.Identity
	signer  Signer
}

func (s *signerWrapper) Sign(message []byte) ([]byte, error) {
	return s.signer.Sign(message)
}

func (s *signerWrapper) Serialize() ([]byte, error) {
	return s.creator, nil
}

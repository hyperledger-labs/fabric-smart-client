/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import "encoding/json"

// SignedMessage is the wire format for an application payload signed with the
// secret key of the sender's comm key-pair. The sender's identity is
// intentionally not part of the message: the receiver learns who the
// counterparty is exclusively from the (transport-authenticated)
// SessionInfo.RemotePKID, which it resolves back to a full identity through
// the endpoint service. This is what binds the verified transport identity to
// the key material used to check the signature.
type SignedMessage struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

// MarshalSignedMessage marshals a SignedMessage wrapping payload and signature.
func MarshalSignedMessage(payload, signature []byte) ([]byte, error) {
	return json.Marshal(&SignedMessage{Payload: payload, Signature: signature})
}

// UnmarshalSignedMessage unmarshals a SignedMessage from its wire format.
func UnmarshalSignedMessage(raw []byte) (*SignedMessage, error) {
	msg := &SignedMessage{}
	if err := json.Unmarshal(raw, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

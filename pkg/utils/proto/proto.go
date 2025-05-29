/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package proto

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

/*
This package delegates the protobuf/proto functionality to `google.golang.org/protobuf/proto` package. This way, we:
- temporarily handle the linting warnings, and
- allow for an easier update of the dependency by updating this single package
*/

type (
	Message   = proto.Message
	MessageV1 = protoadapt.MessageV1
)

// Unmarshal parses the wire-format message in b and places the result in m.
// The provided message must be mutable (e.g., a non-nil pointer to a message).
func Unmarshal(b []byte, m Message) error {
	return proto.Unmarshal(b, m)
}

// UnmarshalV1 parses the wire-format message in b and places the result in m.
// The provided message is a v1 message.
func UnmarshalV1(b []byte, m MessageV1) error {
	return proto.Unmarshal(b, protoadapt.MessageV2Of(m))
}

// Marshal returns the wire-format encoding of m.
func Marshal(m Message) ([]byte, error) {
	return proto.Marshal(m)
}

// MarshalV1 returns the wire-format encoding of m.
// The provided message is a v1 message.
func MarshalV1(m MessageV1) ([]byte, error) {
	return proto.Marshal(protoadapt.MessageV2Of(m))
}

// Equal reports whether two messages are equal,
// by recursively comparing the fields of the message.
func Equal(x, y Message) bool {
	return proto.Equal(x, y)
}

// Clone returns a deep copy of m.
// If the top-level message is invalid, it returns an invalid message as well.
func Clone(src Message) Message {
	return proto.Clone(src)
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package proto

import (
	//lint:ignore SA1019 Dependency to be updated to google.golang.org/protobuf/proto
	protoV1 "github.com/golang/protobuf/proto"
)

/*
This package delegates the protobuf/proto functionality to the deprecated package. This way, we:
- temporarily handle the linting warnings, and
- allow for an easier update of the dependency by updating this single package
*/

type Message = protoV1.Message

func Unmarshal(b []byte, m Message) error {
	return protoV1.Unmarshal(b, m)
}

func Marshal(m Message) ([]byte, error) {
	return protoV1.Marshal(m)
}

func Equal(x, y Message) bool {
	return protoV1.Equal(x, y)
}

func Clone(src Message) Message {
	return protoV1.Clone(src)
}

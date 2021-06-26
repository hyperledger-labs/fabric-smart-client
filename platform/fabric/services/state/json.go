/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"encoding/json"
)

type JSONCodec struct{}

func (J *JSONCodec) Marshal(v interface{}) ([]byte, error) {
	s, ok := v.(Serializable)
	if ok {
		return s.Bytes()
	}
	return json.Marshal(v)
}

func (J *JSONCodec) Unmarshal(data []byte, v interface{}) error {
	s, ok := v.(State)
	if ok {
		return s.SetFromBytes(data)
	}
	return json.Unmarshal(data, v)
}

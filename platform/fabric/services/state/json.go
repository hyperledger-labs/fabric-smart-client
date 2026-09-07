/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"encoding/json"
)

type JSONCodec struct{}

func (j *JSONCodec) Marshal(v any) ([]byte, error) {
	s, ok := v.(Serializable)
	if ok {
		return s.Bytes()
	}
	return json.Marshal(v)
}

func (j *JSONCodec) Unmarshal(data []byte, v any) error {
	s, ok := v.(State)
	if ok {
		return s.SetFromBytes(data)
	}
	return json.Unmarshal(data, v)
}

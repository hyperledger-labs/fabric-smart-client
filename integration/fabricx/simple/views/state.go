/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import "encoding/json"

type SomeObject struct {
	Owner string
	Value int
}

func (s *SomeObject) GetLinearID() (string, error) {
	// in this example we use the object owner as our "linearID" aka world state key
	return s.Owner, nil
}

func Unmarshal(raw []byte) (*SomeObject, error) {
	obj := &SomeObject{}
	err := json.Unmarshal(raw, obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package api

import (
	"bytes"
)

type Scripts struct {
	Birth *Script
	Death *Script
}

// Script models a UTXO script
type Script struct {
	Type string // Unique identifier of this script
	Raw  []byte // Payload of the script. The content depends on the script's type.
}

func (s *Script) Equal(s2 *Script) bool {
	if s2 == nil {
		return false
	}
	return s.Type == s2.Type && bytes.Equal(s.Raw, s2.Raw)
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"encoding/hex"
	"fmt"
	"strings"
)

func EncodeByteA(s string) string {
	return fmt.Sprintf("\\x%s", hex.EncodeToString([]byte(s)))
}

func DecodeByteA(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimLeft(s, "\\x"))
}

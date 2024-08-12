/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"
)

// GenerateBytesUUID returns a UUID based on RFC 4122 returning the generated bytes
func GenerateBytesUUID() []byte {
	uuid := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, uuid)
	if err != nil {
		panic(fmt.Sprintf("Error generating UUID: %s", err))
	}

	// variant bits; see section 4.1.1
	uuid[8] = uuid[8]&^0xc0 | 0x80

	// version 4 (pseudo-random); see section 4.1.3
	uuid[6] = uuid[6]&^0xf0 | 0x40

	return uuid
}

// GenerateUUID returns a UUID based on RFC 4122
func GenerateUUID() string {
	uuid := GenerateBytesUUID()
	return idBytesToStr(uuid)
}

// GenerateUUIDOnlyLetters returns a UUID without digits
func GenerateUUIDOnlyLetters() string {
	// Generate a standard UUID
	uuidWithDigits := GenerateUUID()
	uuidStr := strings.ReplaceAll(uuidWithDigits, "-", "")
	replaceDigits := func(r rune) rune {
		if r >= '0' && r <= '9' {
			// Generate a random letter between A-Z or a-z
			var b [1]byte
			_, err := rand.Read(b[:])
			if err != nil {
				panic(err)
			}
			return rune('a' + (b[0] % 26))
		}
		return r
	}
	return strings.Map(replaceDigits, uuidStr)
}

func idBytesToStr(id []byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:])
}

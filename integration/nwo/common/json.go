/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"encoding/json"

	. "github.com/onsi/gomega"
)

func JSONMarshall(v interface{}) []byte {
	raw, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	return raw
}

func JSONUnmarshal(raw []byte, v interface{}) interface{} {
	err := json.Unmarshal(raw, v)
	Expect(err).NotTo(HaveOccurred())
	return v
}

func JSONUnmarshalString(v interface{}) string {
	var s string
	err := json.Unmarshal(v.([]byte), &s)
	Expect(err).NotTo(HaveOccurred())
	return s
}

func JSONUnmarshalInt(v interface{}) int {
	var s int
	err := json.Unmarshal(v.([]byte), &s)
	Expect(err).NotTo(HaveOccurred())
	return s
}

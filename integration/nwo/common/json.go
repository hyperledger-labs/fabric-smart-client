/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"encoding/json"
	"fmt"

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
	switch vv := v.(type) {
	case []byte:
		err := json.Unmarshal(vv, &s)
		Expect(err).NotTo(HaveOccurred())
	case string:
		err := json.Unmarshal([]byte(vv), &s)
		Expect(err).NotTo(HaveOccurred())
	}
	return s
}

func JSONUnmarshalInt(v interface{}) int {
	var s int
	switch v.(type) {
	case []byte:
		err := json.Unmarshal(v.([]byte), &s)
		Expect(err).NotTo(HaveOccurred())
	case string:
		err := json.Unmarshal([]byte(v.(string)), &s)
		Expect(err).NotTo(HaveOccurred())
	default:
		panic(fmt.Sprintf("type not recognized [%T]", v))
	}
	return s
}

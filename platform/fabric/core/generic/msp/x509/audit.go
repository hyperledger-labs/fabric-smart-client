/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import "encoding/json"

type AuditInfo struct {
	Attributes      [][]byte //revocationhandle
}

func (a *AuditInfo) Bytes() ([]byte, error) {
	return json.Marshal(a)
}

func (a *AuditInfo) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, a)
}

func (a *AuditInfo) EnrollmentID() string {
	return string(a.Attributes[0])
}

//RevocationHandle
func (a *AuditInfo) RevocationHandle() string {
	return string(a.Attributes[1])
}
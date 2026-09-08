/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import "encoding/json"

type AuditInfo struct {
	//nolint:revive // var-naming: renaming this exported struct field is an API break; see follow-up
	EnrollmentId     string
	RevocationHandle []byte
}

func (a *AuditInfo) Bytes() ([]byte, error) {
	return json.Marshal(a)
}

func (a *AuditInfo) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, a)
}

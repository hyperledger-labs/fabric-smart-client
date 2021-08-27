/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"encoding/json"

	csp "github.com/IBM/idemix/bccsp/schemes"
)

type AuditInfo struct {
	*csp.NymEIDAuditData
	Attributes [][]byte
}

func (a *AuditInfo) Bytes() ([]byte, error) {
	return json.Marshal(a)
}

func (a *AuditInfo) FromBytes(raw []byte) error {
	return json.Unmarshal(raw, a)
}

func (a *AuditInfo) EnrollmentID() string {
	return string(a.Attributes[2])
}

func (a *AuditInfo) Match(id []byte) error {
	// si := &m.SerializedIdentity{}
	// err := proto.Unmarshal(id, si)
	// if err != nil {
	// 	return errors.Wrap(err, "failed to unmarshal to msp.SerializedIdentity{}")
	// }

	// serialized := new(m.SerializedIdemixIdentity)
	// err = proto.Unmarshal(si.IdBytes, serialized)
	// if err != nil {
	// 	return errors.Wrap(err, "could not deserialize a SerializedIdemixIdentity")
	// }

	// if _, err := crypto.VerifyAuditingInfo(serialized.Proof, a.IdemixSignatureInfo); err != nil {
	// 	return err
	// }
	return nil
}

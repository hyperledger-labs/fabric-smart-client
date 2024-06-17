/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"encoding/json"

	csp "github.com/IBM/idemix/bccsp/types"
	im "github.com/IBM/idemix/idemixmsp"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"
)

type AuditInfo struct {
	EidNymAuditData *csp.AttrNymAuditData
	RhNymAuditData  *csp.AttrNymAuditData
	Attributes      [][]byte

	Csp           csp.BCCSP     `json:"-"`
	Ipk           []byte        `json:"-"`
	SchemaManager SchemaManager `json:"-"`
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

func (a *AuditInfo) RevocationHandle() string {
	return string(a.Attributes[3])
}

func (a *AuditInfo) Match(id []byte) error {
	si := &m.SerializedIdentity{}
	err := proto.Unmarshal(id, si)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal to msp.SerializedIdentity{}")
	}

	serialized := new(im.SerializedIdemixIdentity)
	err = proto.Unmarshal(si.IdBytes, serialized)
	if err != nil {
		return errors.Wrap(err, "could not deserialize a SerializedIdemixIdentity")
	}

	opts, err := a.SchemaManager.PublicKeyImportOpts(string(serialized.Schema))
	if err != nil {
		return errors.Wrapf(err, "could not obtain PublicKeyImportOpts for schema '%s'", string(serialized.Schema))
	}

	issuerPublicKey, err := a.Csp.KeyImport(
		a.Ipk,
		opts)
	if err != nil {
		return errors.Wrap(err, "could not obtain import public key")
	}

	eidOpts, err := a.SchemaManager.EidNymAuditOpts(serialized, a.Attributes)
	if err != nil {
		return errors.Wrapf(err, "could not obtain EidNymAuditOpts for schema '%s'", string(serialized.Schema))
	}
	eidOpts.RNymEid = a.EidNymAuditData.Rand

	// Audit EID
	valid, err := a.Csp.Verify(
		issuerPublicKey,
		serialized.Proof,
		nil,
		eidOpts,
	)
	if err != nil {
		return errors.Wrap(err, "error while verifying the nym eid")
	}
	if !valid {
		return errors.New("invalid nym rh")
	}

	rhOpts, err := a.SchemaManager.RhNymAuditOpts(serialized, a.Attributes)
	if err != nil {
		return errors.Wrapf(err, "could not obtain EidNymAuditOpts for schema '%s'", string(serialized.Schema))
	}
	rhOpts.RNymRh = a.RhNymAuditData.Rand

	// Audit RH
	valid, err = a.Csp.Verify(
		issuerPublicKey,
		serialized.Proof,
		nil,
		rhOpts,
	)
	if err != nil {
		return errors.Wrap(err, "error while verifying the nym rh")
	}
	if !valid {
		return errors.New("invalid nym eid")
	}

	return nil
}

func DeserializeAuditInfo(raw []byte) (*AuditInfo, error) {
	auditInfo := &AuditInfo{}
	err := auditInfo.FromBytes(raw)
	if err != nil {
		return nil, err
	}
	return auditInfo, nil
}

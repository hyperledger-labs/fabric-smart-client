/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"fmt"

	msp "github.com/IBM/idemix"
	bccsp "github.com/IBM/idemix/bccsp/types"
	im "github.com/IBM/idemix/idemixmsp"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"
)

// SchemaManager handles the various credential schemas. A credential schema
// contains information about the number of attributes, which attributes
// must be disclosed when creating proofs, the format of the attributes etc.
type SchemaManager interface {
	// PublicKeyImportOpts returns the options that `schema` uses to import its public keys
	PublicKeyImportOpts(schema string) (*bccsp.IdemixIssuerPublicKeyImportOpts, error)
	// SignerOpts returns the options that `sid` must use to generate a signature (ZKP)
	SignerOpts(sid *im.SerializedIdemixIdentity) (*bccsp.IdemixSignerOpts, error)
	// NymSignerOpts returns the options that `schema` uses to verify a nym signature
	NymSignerOpts(schema string) (*bccsp.IdemixNymSignerOpts, error)
	// EidNymAuditOpts returns the options that `sid` must use to audit an EIDNym
	EidNymAuditOpts(sid *im.SerializedIdemixIdentity, attrs [][]byte) (*bccsp.EidNymAuditOpts, error)
	// RhNymAuditOpts returns the options that `sid` must use to audit an RhNym
	RhNymAuditOpts(sid *im.SerializedIdemixIdentity, attrs [][]byte) (*bccsp.RhNymAuditOpts, error)
}

const (
	eidIdx = 2
	rhIdx  = 3
	skIdx  = 0
)

// defaultSchemaManager implements the default schema for fabric:
// - 4 attributes (OU, Role, EID, RH)
// - all in bytes format except for Role
// - fixed positions
// - no other attributes
// - a "hidden" usk attribute at position 0
type defaultSchemaManager struct {
}

// attributeNames are the attribute names for the `w3c` schema
var attributeNames = []string{
	"_:c14n0 <http://www.w3.",
	"_:c14n0 <https://w3id.o",
	"_:c14n0 <https://w3id.o",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<did:key:z6MknntgQWCT8Z",
	"<https://issuer.oidp.us",
	"<https://issuer.oidp.us",
	"<https://issuer.oidp.us",
	"<https://issuer.oidp.us",
	"<https://issuer.oidp.us",
	"<https://issuer.oidp.us",
	"<https://issuer.oidp.us",
	"_:c14n0 <cbdccard:2_ou>",
	"_:c14n0 <cbdccard:3_rol",
	"_:c14n0 <cbdccard:4_eid",
	"_:c14n0 <cbdccard:5_rh>",
}

func (*defaultSchemaManager) NymSignerOpts(schema string) (*bccsp.IdemixNymSignerOpts, error) {
	switch schema {
	case "":
		return &bccsp.IdemixNymSignerOpts{}, nil
	case "w3c-v0.0.1":
		return &bccsp.IdemixNymSignerOpts{
			SKIndex: 24,
		}, nil
	}

	return nil, fmt.Errorf("unsupported schema '%s' for NymSignerOpts", schema)
}

func (*defaultSchemaManager) PublicKeyImportOpts(schema string) (*bccsp.IdemixIssuerPublicKeyImportOpts, error) {
	switch schema {
	case "":
		return &bccsp.IdemixIssuerPublicKeyImportOpts{
			Temporary: true,
			AttributeNames: []string{
				msp.AttributeNameOU,
				msp.AttributeNameRole,
				msp.AttributeNameEnrollmentId,
				msp.AttributeNameRevocationHandle,
			},
		}, nil
	case "w3c-v0.0.1":
		return &bccsp.IdemixIssuerPublicKeyImportOpts{
			Temporary:      true,
			AttributeNames: append([]string{""}, attributeNames...),
		}, nil
	}

	return nil, fmt.Errorf("unsupported schema '%s' for PublicKeyImportOpts", schema)
}

func (*defaultSchemaManager) RhNymAuditOpts(sid *im.SerializedIdemixIdentity, attrs [][]byte) (*bccsp.RhNymAuditOpts, error) {
	switch sid.Schema {
	case "":
		return &bccsp.RhNymAuditOpts{
			RhIndex:          rhIdx,
			SKIndex:          skIdx,
			RevocationHandle: string(attrs[rhIdx]),
		}, nil
	case "w3c-v0.0.1":
		return &bccsp.RhNymAuditOpts{
			RhIndex:          27,
			SKIndex:          24,
			RevocationHandle: string(attrs[27]),
		}, nil
	}

	return nil, fmt.Errorf("unsupported schema '%s' for NymSignerOpts", sid.Schema)
}

func (*defaultSchemaManager) EidNymAuditOpts(sid *im.SerializedIdemixIdentity, attrs [][]byte) (*bccsp.EidNymAuditOpts, error) {
	switch sid.Schema {
	case "":
		return &bccsp.EidNymAuditOpts{
			EidIndex:     eidIdx,
			SKIndex:      skIdx,
			EnrollmentID: string(attrs[eidIdx]),
		}, nil
	case "w3c-v0.0.1":
		return &bccsp.EidNymAuditOpts{
			EidIndex:     26,
			SKIndex:      24,
			EnrollmentID: string(attrs[26]),
		}, nil
	}

	return nil, fmt.Errorf("unsupported schema '%s' for NymSignerOpts", sid.Schema)
}

func (*defaultSchemaManager) SignerOpts(sid *im.SerializedIdemixIdentity) (*bccsp.IdemixSignerOpts, error) {
	// OU
	ou := &m.OrganizationUnit{}
	err := proto.Unmarshal(sid.Ou, ou)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the OU of the identity")
	}

	// Role
	role := &m.MSPRole{}
	err = proto.Unmarshal(sid.Role, role)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the role of the identity")
	}

	switch sid.Schema {
	case "":
		return &bccsp.IdemixSignerOpts{
			Attributes: []bccsp.IdemixAttribute{
				{Type: bccsp.IdemixBytesAttribute, Value: []byte(ou.OrganizationalUnitIdentifier)},
				{Type: bccsp.IdemixIntAttribute, Value: GetIdemixRoleFromMSPRole(role)},
				{Type: bccsp.IdemixHiddenAttribute},
				{Type: bccsp.IdemixHiddenAttribute},
			},
			RhIndex:  rhIdx,
			EidIndex: eidIdx,
		}, nil
	case "w3c-v0.0.1":
		role_str := fmt.Sprintf("_:c14n0 \u003ccbdccard:3_role\u003e \"%d\"^^\u003chttp://www.w3.org/2001/XMLSchema#integer\u003e .", GetIdemixRoleFromMSPRole(role))

		idemixAttrs := []bccsp.IdemixAttribute{}
		for i := range attributeNames {
			if i == 25 {
				idemixAttrs = append(idemixAttrs, bccsp.IdemixAttribute{
					Type:  bccsp.IdemixBytesAttribute,
					Value: []byte(role_str),
				})
			} else if i == 24 {
				idemixAttrs = append(idemixAttrs, bccsp.IdemixAttribute{
					Type:  bccsp.IdemixBytesAttribute,
					Value: []byte(ou.OrganizationalUnitIdentifier),
				})
			} else {
				idemixAttrs = append(idemixAttrs, bccsp.IdemixAttribute{
					Type: bccsp.IdemixHiddenAttribute,
				})
			}
		}

		return &bccsp.IdemixSignerOpts{
			Attributes:       idemixAttrs,
			RhIndex:          27,
			EidIndex:         26,
			SKIndex:          24,
			VerificationType: bccsp.ExpectEidNymRhNym,
		}, nil
	}

	return nil, fmt.Errorf("unsupported schema '%s' for NymSignerOpts", sid.Schema)
}

type Identity struct {
	Identity           *MSPIdentity
	NymPublicKey       bccsp.Key
	SerializedIdentity *m.SerializedIdentity
	OU                 *m.OrganizationUnit
	Role               *m.MSPRole
	Schema             string
}

type Idemix struct {
	Name            string
	Ipk             []byte
	Csp             bccsp.BCCSP
	IssuerPublicKey bccsp.Key
	RevocationPK    bccsp.Key
	Epoch           int
	VerType         bccsp.VerificationType
	NymEID          []byte
	RhNym           []byte

	SchemaManager SchemaManager
}

func (c *Idemix) Deserialize(raw []byte, checkValidity bool) (*Identity, error) {
	return c.DeserializeAgainstNymEID(raw, checkValidity, nil)
}

func (c *Idemix) DeserializeAgainstNymEID(raw []byte, checkValidity bool, nymEID []byte) (*Identity, error) {
	si := &m.SerializedIdentity{}
	err := proto.Unmarshal(raw, si)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal to msp.SerializedIdentity{}")
	}

	serialized := new(im.SerializedIdemixIdentity)
	err = proto.Unmarshal(si.IdBytes, serialized)
	if err != nil {
		return nil, errors.Wrap(err, "could not deserialize a SerializedIdemixIdentity")
	}
	if serialized.NymX == nil || serialized.NymY == nil {
		return nil, errors.Errorf("unable to deserialize idemix identity: pseudonym is invalid")
	}

	// Import NymPublicKey
	var rawNymPublicKey []byte
	rawNymPublicKey = append(rawNymPublicKey, serialized.NymX...)
	rawNymPublicKey = append(rawNymPublicKey, serialized.NymY...)
	NymPublicKey, err := c.Csp.KeyImport(
		rawNymPublicKey,
		&bccsp.IdemixNymPublicKeyImportOpts{Temporary: true},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to import nym public key")
	}

	// OU
	ou := &m.OrganizationUnit{}
	err = proto.Unmarshal(serialized.Ou, ou)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the OU of the identity")
	}

	// Role
	role := &m.MSPRole{}
	err = proto.Unmarshal(serialized.Role, role)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize the role of the identity")
	}

	if len(c.Ipk) != 0 {
		opts, err := c.SchemaManager.PublicKeyImportOpts(string(serialized.Schema))
		if err != nil {
			return nil, errors.Wrapf(err, "could not obtain PublicKeyImportOpts for schema '%s'", string(serialized.Schema))
		}

		c.IssuerPublicKey, err = c.Csp.KeyImport(c.Ipk, opts)
		if err != nil {
			return nil, errors.Wrap(err, "could not obtain import public key")
		}
	}

	idemix := c
	if len(nymEID) != 0 {
		idemix = &Idemix{
			Name:            c.Name,
			Ipk:             c.Ipk,
			Csp:             c.Csp,
			IssuerPublicKey: c.IssuerPublicKey,
			RevocationPK:    c.RevocationPK,
			Epoch:           c.Epoch,
			VerType:         c.VerType,
			NymEID:          nymEID,
		}
	}

	id, err := newMSPIdentityWithVerType(
		idemix,
		NymPublicKey,
		serialized,
		role,
		ou,
		serialized.Proof,
		c.VerType,
		c.SchemaManager,
	)
	if err != nil {
		return nil, errors.Wrap(err, "cannot deserialize")
	}
	if checkValidity {
		if err := id.Validate(); err != nil {
			return nil, errors.Wrap(err, "cannot deserialize, invalid identity")
		}
	}

	return &Identity{
		Identity:           id,
		NymPublicKey:       NymPublicKey,
		SerializedIdentity: si,
		OU:                 ou,
		Role:               role,
		Schema:             serialized.Schema,
	}, nil
}

func (c *Idemix) DeserializeAuditInfo(raw []byte) (*AuditInfo, error) {
	ai, err := DeserializeAuditInfo(raw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed deserializing audit info [%s]", string(raw))
	}
	ai.Csp = c.Csp
	ai.Ipk = c.Ipk
	ai.SchemaManager = c.SchemaManager
	return ai, nil
}

type NymSignatureVerifier struct {
	CSP   bccsp.BCCSP
	IPK   bccsp.Key
	NymPK bccsp.Key

	SchemaManager SchemaManager
	Schema        string
}

func (v *NymSignatureVerifier) Verify(message, sigma []byte) error {
	opts, err := v.SchemaManager.NymSignerOpts(v.Schema)
	if err != nil {
		return errors.Wrapf(err, "could not obtain NymSignerOpts for schema '%s'", v.Schema)
	}

	opts.IssuerPK = v.IPK

	_, err = v.CSP.Verify(
		v.NymPK,
		sigma,
		message,
		opts,
	)
	return err
}

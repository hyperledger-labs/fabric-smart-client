/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
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
}

const (
	eidIdx = 2
	rhIdx  = 3
)

// defaultSchemaManager implements the default schema for fabric:
// - 4 attributes (OU, Role, EID, RH)
// - all in bytes format except for Role
// - fixed positions
// - no other attributes
// - a "hidden" usk attribute at position 0
type defaultSchemaManager struct {
}

func (*defaultSchemaManager) PublicKeyImportOpts(schema string) (*bccsp.IdemixIssuerPublicKeyImportOpts, error) {
	return &bccsp.IdemixIssuerPublicKeyImportOpts{
		Temporary: true,
		AttributeNames: []string{
			msp.AttributeNameOU,
			msp.AttributeNameRole,
			msp.AttributeNameEnrollmentId,
			msp.AttributeNameRevocationHandle,
		},
	}, nil
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
}

type Identity struct {
	Identity           *MSPIdentity
	NymPublicKey       bccsp.Key
	SerializedIdentity *m.SerializedIdentity
	OU                 *m.OrganizationUnit
	Role               *m.MSPRole
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
}

func (v *NymSignatureVerifier) Verify(message, sigma []byte) error {
	_, err := v.CSP.Verify(
		v.NymPK,
		sigma,
		message,
		&bccsp.IdemixNymSignerOpts{
			IssuerPK: v.IPK,
		},
	)
	return err
}

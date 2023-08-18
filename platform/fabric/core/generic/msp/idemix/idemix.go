/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	bccsp "github.com/IBM/idemix/bccsp/types"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"
)

type Identity struct {
	id           *MSPIdentity
	NymPublicKey bccsp.Key
	si           *m.SerializedIdentity
	ou           *m.OrganizationUnit
	role         *m.MSPRole
}

type Idemix struct {
	name            string
	Ipk             []byte
	Csp             bccsp.BCCSP
	IssuerPublicKey bccsp.Key
	revocationPK    bccsp.Key
	epoch           int
	VerType         bccsp.VerificationType
	NymEID          []byte
	RhNym           []byte
}

func (s *Idemix) Deserialize(raw []byte, checkValidity bool) (*Identity, error) {
	si := &m.SerializedIdentity{}
	err := proto.Unmarshal(raw, si)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal to msp.SerializedIdentity{}")
	}

	serialized := new(m.SerializedIdemixIdentity)
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
	NymPublicKey, err := s.Csp.KeyImport(
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

	id := newIdentityWithVerType(s, NymPublicKey, role, ou, serialized.Proof, s.VerType)
	if checkValidity {
		if err := id.Validate(); err != nil {
			return nil, errors.Wrap(err, "cannot deserialize, invalid identity")
		}
	}

	return &Identity{
		id:           id,
		NymPublicKey: NymPublicKey,
		si:           si,
		ou:           ou,
		role:         role,
	}, nil
}

func (s *Idemix) DeserializeAuditInfo(raw []byte) (*AuditInfo, error) {
	ai := &AuditInfo{
		Csp:             s.Csp,
		IssuerPublicKey: s.IssuerPublicKey,
	}
	if err := ai.FromBytes(raw); err != nil {
		return nil, errors.Wrapf(err, "failed deserializing audit info [%s]", string(raw))
	}
	return ai, nil
}

type Verifier struct {
	idd          *Deserializer
	nymPublicKey bccsp.Key
}

func (v *Verifier) Verify(message, sigma []byte) error {
	_, err := v.idd.Csp.Verify(
		v.nymPublicKey,
		sigma,
		message,
		&bccsp.IdemixNymSignerOpts{
			IssuerPK: v.idd.IssuerPublicKey,
		},
	)
	return err
}

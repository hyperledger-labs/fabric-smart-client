/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"fmt"

	csp "github.com/IBM/idemix/bccsp/types"
	math "github.com/IBM/mathlib"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type Deserializer struct {
	*Idemix
}

// NewDeserializer returns a new deserializer for the best effort strategy
func NewDeserializer(ipk []byte) (*Deserializer, error) {
	bccsp, err := NewBCCSP(math.FP256BN_AMCL)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to instantiate bccsp")
	}
	return NewDeserializerWithBCCSP(ipk, csp.BestEffort, nil, bccsp)
}

func NewDeserializerWithBCCSP(ipk []byte, verType csp.VerificationType, nymEID []byte, cryptoProvider csp.BCCSP) (*Deserializer, error) {
	return NewDeserializerWithBCCSPSchema(ipk, verType, nymEID, cryptoProvider, &defaultSchemaManager{})
}

func NewDeserializerWithBCCSPSchema(ipk []byte, verType csp.VerificationType,
	nymEID []byte, cryptoProvider csp.BCCSP, sm SchemaManager) (*Deserializer, error) {
	logger.Debugf("Setting up Idemix-based MSP instance")

	return &Deserializer{
		Idemix: &Idemix{
			Ipk:           ipk,
			Csp:           cryptoProvider,
			VerType:       verType,
			NymEID:        nymEID,
			SchemaManager: sm,
		},
	}, nil
}

func (i *Deserializer) DeserializeVerifier(raw []byte) (driver.Verifier, error) {
	identity, err := i.Deserialize(raw, true)
	if err != nil {
		return nil, err
	}

	return &NymSignatureVerifier{
		CSP:   i.Idemix.Csp,
		IPK:   i.Idemix.IssuerPublicKey,
		NymPK: identity.NymPublicKey,
	}, nil
}

func (i *Deserializer) DeserializeVerifierAgainstNymEID(raw []byte, nymEID []byte) (driver.Verifier, error) {
	identity, err := i.Idemix.DeserializeAgainstNymEID(raw, true, nymEID)
	if err != nil {
		return nil, err
	}

	return &NymSignatureVerifier{
		CSP:   i.Idemix.Csp,
		IPK:   i.Idemix.IssuerPublicKey,
		NymPK: identity.NymPublicKey,
	}, nil
}

func (i *Deserializer) DeserializeSigner(raw []byte) (driver.Signer, error) {
	return nil, errors.New("not supported")
}

func (i *Deserializer) DeserializeAuditInfo(raw []byte) (*AuditInfo, error) {
	return i.Idemix.DeserializeAuditInfo(raw)
}

func (i *Deserializer) Info(raw []byte, auditInfo []byte) (string, error) {
	r, err := i.Deserialize(raw, false)
	if err != nil {
		return "", err
	}

	eid := ""
	if len(auditInfo) != 0 {
		ai, err := DeserializeAuditInfo(auditInfo)
		if err != nil {
			return "", err
		}

		ai.SchemaManager = i.SchemaManager

		if err := ai.Match(raw); err != nil {
			return "", err
		}
		eid = ai.EnrollmentID()
	}

	return fmt.Sprintf("MSP.Idemix: [%s][%s][%s][%s][%s]", eid, view.Identity(raw).UniqueID(), r.SerializedIdentity.Mspid, r.OU.OrganizationalUnitIdentifier, r.Role.Role.String()), nil
}

func (i *Deserializer) String() string {
	return fmt.Sprintf("Idemix with IPK [%s]", hash.Hashable(i.Ipk).String())
}

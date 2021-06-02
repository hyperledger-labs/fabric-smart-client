/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix

import (
	"fmt"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/csp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type idd struct {
	*support
}

func NewDeserializer(ipk []byte) (*idd, error) {
	logger.Debugf("Setting up Idemix-based MSP instance")

	cryptoProvider, err := idemix.New(sw.NewDummyKeyStore())
	if err != nil {
		return nil, errors.Wrap(err, "failed getting crypto provider")
	}

	// Import Issuer Public Key
	var issuerPublicKey bccsp.Key
	if len(ipk) != 0 {
		issuerPublicKey, err = cryptoProvider.KeyImport(
			ipk,
			&csp.IdemixIssuerPublicKeyImportOpts{
				Temporary: true,
				AttributeNames: []string{
					msp.AttributeNameOU,
					msp.AttributeNameRole,
					msp.AttributeNameEnrollmentId,
					msp.AttributeNameRevocationHandle,
				},
			})
		if err != nil {
			return nil, err
		}
	}

	return &idd{
		&support{
			ipk:             ipk,
			csp:             cryptoProvider,
			issuerPublicKey: issuerPublicKey,
		},
	}, nil
}

func (i *idd) DeserializeVerifier(raw []byte) (api.Verifier, error) {
	r, err := i.Deserialize(raw, false)
	if err != nil {
		return nil, err
	}

	return &verifier{
		idd:          i,
		nymPublicKey: r.NymPublicKey,
	}, nil
}

func (i *idd) DeserializeSigner(raw []byte) (api.Signer, error) {
	return nil, errors.New("not supported")
}

func (i *idd) Info(raw []byte, auditInfo []byte) (string, error) {
	r, err := i.Deserialize(raw, false)
	if err != nil {
		return "", err
	}

	eid := ""
	if len(auditInfo) != 0 {
		ai := &AuditInfo{}
		if err := ai.FromBytes(auditInfo); err != nil {
			return "", err
		}
		if err := ai.Match(view.Identity(raw)); err != nil {
			return "", err
		}
		eid = ai.EnrollmentID()
	}

	return fmt.Sprintf("MSP.Idemix: [%s][%s][%s][%s][%s]", eid, view.Identity(raw).UniqueID(), r.si.Mspid, r.ou.OrganizationalUnitIdentifier, r.role.Role.String()), nil
}

func (i *idd) String() string {
	return fmt.Sprintf("Idemix with IPK [%s]", hash.Hashable(i.ipk).String())
}

type verifier struct {
	idd          *idd
	nymPublicKey bccsp.Key
}

func (v *verifier) Verify(message, sigma []byte) error {
	_, err := v.idd.csp.Verify(
		v.nymPublicKey,
		sigma,
		message,
		&csp.IdemixNymSignerOpts{
			IssuerPK: v.idd.issuerPublicKey,
		},
	)
	return err
}

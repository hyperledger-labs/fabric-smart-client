/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package msp

import (
	msp2 "github.com/IBM/idemix/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type idemixSigningIdentityWrapper struct {
	*msp2.IdemixSigningIdentity
}

func (i *idemixSigningIdentityWrapper) GetPublicVersion() Identity {
	pv := i.IdemixSigningIdentity.GetPublicVersion()
	pub, ok := pv.(*msp2.Idemixidentity)
	if !ok {
		panic(errors.Errorf("unexpected public identity type [%T]", pv))
	}
	return &idemixIdentityWrapper{Idemixidentity: pub}
}

func (i *idemixSigningIdentityWrapper) GetIdentifier() *IdentityIdentifier {
	return i.GetPublicVersion().GetIdentifier()
}

func (i *idemixSigningIdentityWrapper) GetOrganizationalUnits() []*OUIdentifier {
	return i.GetPublicVersion().GetOrganizationalUnits()
}

type idemixIdentityWrapper struct {
	*msp2.Idemixidentity
}

func (i *idemixIdentityWrapper) GetIdentifier() *IdentityIdentifier {
	id := i.Idemixidentity.GetIdentifier()

	return &IdentityIdentifier{
		Mspid: id.Mspid,
		Id:    id.Id,
	}
}

func (i *idemixIdentityWrapper) GetOrganizationalUnits() []*OUIdentifier {
	ous := i.Idemixidentity.GetOrganizationalUnits()
	wous := []*OUIdentifier{}
	for _, ou := range ous {
		wous = append(wous, &OUIdentifier{
			CertifiersIdentifier:         ou.CertifiersIdentifier,
			OrganizationalUnitIdentifier: ou.OrganizationalUnitIdentifier,
		})
	}

	return wous
}

type idemixMSPWrapper struct {
	*msp2.Idemixmsp
}

func (i *idemixMSPWrapper) deserializeIdentityInternal(serializedIdentity []byte) (Identity, error) {
	id, err := i.DeserializeIdentityInternal(serializedIdentity)
	if err != nil {
		return nil, err
	}

	idemixID, ok := id.(*msp2.Idemixidentity)
	if !ok {
		return nil, errors.Errorf("unexpected identity type [%T]", id)
	}
	return &idemixIdentityWrapper{idemixID}, nil
}

func (i *idemixMSPWrapper) DeserializeIdentity(serializedIdentity []byte) (Identity, error) {
	id, err := i.Idemixmsp.DeserializeIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}

	idemixID, ok := id.(*msp2.Idemixidentity)
	if !ok {
		return nil, errors.Errorf("unexpected identity type [%T]", id)
	}
	return &idemixIdentityWrapper{idemixID}, nil
}

func (i *idemixMSPWrapper) GetVersion() MSPVersion {
	return MSPVersion(i.Idemixmsp.GetVersion())
}

func (i *idemixMSPWrapper) GetType() ProviderType {
	return ProviderType(i.Idemixmsp.GetType())
}

func (i *idemixMSPWrapper) GetDefaultSigningIdentity() (SigningIdentity, error) {
	id, err := i.Idemixmsp.GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}

	signingID, ok := id.(*msp2.IdemixSigningIdentity)
	if !ok {
		return nil, errors.Errorf("unexpected signing identity type [%T]", id)
	}
	return &idemixSigningIdentityWrapper{signingID}, nil
}

func (i *idemixMSPWrapper) Validate(id Identity) error {
	wrapped, ok := id.(*idemixIdentityWrapper)
	if !ok {
		return errors.Errorf("unexpected identity type [%T]", id)
	}
	return i.Idemixmsp.Validate(wrapped.Idemixidentity)
}

func (i *idemixMSPWrapper) SatisfiesPrincipal(id Identity, principal *msp.MSPPrincipal) error {
	wrapped, ok := id.(*idemixIdentityWrapper)
	if !ok {
		return errors.Errorf("unexpected identity type [%T]", id)
	}
	return i.Idemixmsp.SatisfiesPrincipal(wrapped.Idemixidentity, principal)
}

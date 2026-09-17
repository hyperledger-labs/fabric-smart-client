/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package msp

import (
	msp2 "github.com/IBM/idemix/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

type idemixSigningIdentityWrapper struct {
	msp2.SigningIdentity
}

func (i *idemixSigningIdentityWrapper) GetPublicVersion() Identity {
	pv := i.SigningIdentity.GetPublicVersion()
	return &idemixIdentityWrapper{pv}
}

func (i *idemixSigningIdentityWrapper) GetIdentifier() *IdentityIdentifier {
	return i.GetPublicVersion().GetIdentifier()
}

func (i *idemixSigningIdentityWrapper) GetOrganizationalUnits() []*OUIdentifier {
	return i.GetPublicVersion().GetOrganizationalUnits()
}

type idemixIdentityWrapper struct {
	msp2.Identity
}

func (i *idemixIdentityWrapper) GetIdentifier() *IdentityIdentifier {
	id := i.Identity.GetIdentifier()

	return &IdentityIdentifier{
		Mspid: id.Mspid,
		Id:    id.Id,
	}
}

func (i *idemixIdentityWrapper) GetOrganizationalUnits() []*OUIdentifier {
	ous := i.Identity.GetOrganizationalUnits()
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
	*msp2.MSP
}

// deserializeIdentityInternal deserializes an identity given only the idemix-specific
// payload (the IdBytes of a SerializedIdentity), skipping the outer MSP-id check the
// caller already performed. msp2.MSP does not expose this shortcut, so it is
// reconstructed by re-wrapping the payload into a SerializedIdentity and going through
// the public DeserializeIdentity path.
func (i *idemixMSPWrapper) deserializeIdentityInternal(serializedIdentity []byte) (Identity, error) {
	mspID, err := i.MSP.GetIdentifier()
	if err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(&msp.SerializedIdentity{Mspid: mspID, IdBytes: serializedIdentity})
	if err != nil {
		return nil, errors.Wrap(err, "failed marshalling serialized identity")
	}
	id, err := i.MSP.DeserializeIdentity(raw)
	if err != nil {
		return nil, err
	}
	return &idemixIdentityWrapper{id}, nil
}

func (i *idemixMSPWrapper) DeserializeIdentity(serializedIdentity []byte) (Identity, error) {
	id, err := i.MSP.DeserializeIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}
	return &idemixIdentityWrapper{id}, nil
}

func (i *idemixMSPWrapper) GetVersion() MSPVersion {
	return MSPVersion(i.MSP.GetVersion())
}

func (i *idemixMSPWrapper) GetType() ProviderType {
	return ProviderType(i.MSP.GetType())
}

func (i *idemixMSPWrapper) GetDefaultSigningIdentity() (SigningIdentity, error) {
	id, err := i.MSP.GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}
	return &idemixSigningIdentityWrapper{id}, nil
}

func (i *idemixMSPWrapper) Validate(id Identity) error {
	wrapped, ok := id.(*idemixIdentityWrapper)
	if !ok {
		return errors.Errorf("unexpected identity type [%T]", id)
	}
	return i.MSP.Validate(wrapped.Identity)
}

func (i *idemixMSPWrapper) SatisfiesPrincipal(id Identity, principal *msp.MSPPrincipal) error {
	wrapped, ok := id.(*idemixIdentityWrapper)
	if !ok {
		return errors.Errorf("unexpected identity type [%T]", id)
	}
	return i.MSP.SatisfiesPrincipal(wrapped.Identity, principal)
}

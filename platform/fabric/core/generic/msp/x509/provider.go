/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	api2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"
)

type SignerService interface {
	RegisterSigner(identity view.Identity, signer api2.Signer, verifier api2.Verifier) error
}

type provider struct {
	sID          SigningIdentity
	id           []byte
	enrollmentID string
}

func NewProvider(mspConfigPath, mspID string, signerService SignerService) (*provider, error) {
	return NewProviderWithBCCSPConfig(mspConfigPath, mspID, signerService, nil)
}

func NewProviderWithBCCSPConfig(mspConfigPath, mspID string, signerService SignerService, bccspConfig *config.BCCSP) (*provider, error) {
	sID, err := GetSigningIdentity(mspConfigPath, mspID, bccspConfig)
	if err != nil {
		return nil, err
	}
	idRaw, err := sID.Serialize()
	if err != nil {
		return nil, err
	}
	if signerService != nil {
		err = signerService.RegisterSigner(idRaw, sID, sID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed registering x509 signer")
		}
	}
	enrollmentID, err := GetEnrollmentID(idRaw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting enrollment id for [%s:%s]", mspConfigPath, mspID)
	}

	return &provider{sID: sID, id: idRaw, enrollmentID: enrollmentID}, nil
}

func (p *provider) Identity(opts *api2.IdentityOptions) (view.Identity, []byte, error) {
	return p.id, []byte(p.enrollmentID), nil
}

func (p *provider) EnrollmentID() string {
	return p.enrollmentID
}

func (p *provider) DeserializeVerifier(raw []byte) (driver.Verifier, error) {
	si := &msp.SerializedIdentity{}
	err := proto.Unmarshal(raw, si)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal to msp.SerializedIdentity{}")
	}
	genericPublicKey, err := PemDecodeKey(si.IdBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed parsing received public key")
	}
	publicKey, ok := genericPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("expected *ecdsa.PublicKey")
	}

	// TODO: check the validity of the identity against the msp

	return NewVerifier(publicKey), nil
}

func (p *provider) DeserializeSigner(raw []byte) (driver.Signer, error) {
	return nil, errors.New("not supported")
}

func (p *provider) Info(raw []byte, auditInfo []byte) (string, error) {
	si := &msp.SerializedIdentity{}
	err := proto.Unmarshal(raw, si)
	if err != nil {
		return "", err
	}
	cert, err := PemDecodeCert(si.IdBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MSP.x509: [%s][%s][%s]", view.Identity(raw).UniqueID(), si.Mspid, cert.Subject.CommonName), nil
}

func (p *provider) SerializedIdentity() (SigningIdentity, error) {
	return p.sID, nil
}

func (p *provider) String() string {
	return fmt.Sprintf("X509 Provider for EID [%s]", p.enrollmentID)
}

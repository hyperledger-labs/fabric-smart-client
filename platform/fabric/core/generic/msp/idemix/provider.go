/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"context"
	"fmt"

	idemixmsp "github.com/IBM/idemix/msp"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	mspdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

type KVS interface {
	Exists(ctx context.Context, id string) bool
	Put(ctx context.Context, id string, state any) error
	Get(ctx context.Context, id string, state any) error
}

type kvsAdapter struct {
	kvs KVS
}

func (k *kvsAdapter) Put(id string, state any) error {
	return k.kvs.Put(context.Background(), id, state)
}

func (k *kvsAdapter) Get(id string, state any) error {
	return k.kvs.Get(context.Background(), id, state)
}

type msp interface {
	Pseudonym() (idemixmsp.SigningIdentity, []byte, error)
	DeserializeIdentity(serializedID []byte) (idemixmsp.Identity, error)
	DeserializeSigningIdentity(raw []byte) (idemixmsp.SigningIdentity, error)
	EnrollmentID() string
	IssuerPublicKey() []byte
}

type Provider struct {
	msp           msp
	signerService mspdriver.SignerService
}

func NewProvider(conf1 *m.MSPConfig, kvs KVS, signerService mspdriver.SignerService) (*Provider, error) {
	msp, err := idemixmsp.NewIdemixMspWithKeyStore(idemixmsp.MSPv1_4_3, nil, &kvsAdapter{kvs: kvs})
	if err != nil {
		return nil, errors.Wrap(err, "failed creating MSP")
	}
	if err := msp.Setup(conf1); err != nil {
		return nil, errors.Wrap(err, "failed setting up MSP")
	}

	return &Provider{
		msp:           msp,
		signerService: signerService,
	}, nil
}

func (p *Provider) DeserializeVerifier(raw []byte) (driver.Verifier, error) {
	identity, err := p.msp.DeserializeIdentity(raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed deserializing identity")
	}
	if err := identity.Validate(); err != nil {
		return nil, errors.Wrap(err, "failed validating deserialized identity")
	}

	return identity, nil
}

func (p *Provider) DeserializeSigner(raw []byte) (driver.Signer, error) {
	si, err := p.msp.DeserializeSigningIdentity(raw)
	if err != nil {
		return nil, errors.Wrap(err, "failed deserializing identity")
	}
	if err := si.Validate(); err != nil {
		return nil, errors.Wrap(err, "failed validating deserialized identity")
	}

	msg := []byte("hello world!!!")
	sigma, err := si.Sign(msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed generating verification signature")
	}
	if err := si.Verify(msg, sigma); err != nil {
		return nil, errors.Wrap(err, "failed verifying verification signature")
	}
	return si, nil
}

func (p *Provider) Info(raw, _ []byte) (string, error) {
	identity, err := p.msp.DeserializeIdentity(raw)
	if err != nil {
		return "", errors.Wrap(err, "failed deserializing identity")
	}

	ous := identity.GetOrganizationalUnits()
	ou := ""
	if len(ous) > 0 {
		ou = ous[0].OrganizationalUnitIdentifier
	}

	return fmt.Sprintf("MSP.Idemix: [%s][%s][%s]", view.Identity(raw).UniqueID(), identity.GetMSPIdentifier(), ou), nil
}

func (p *Provider) String() string {
	return fmt.Sprintf("Idemix Provider [%s]", logging.SHA256Base64(p.msp.IssuerPublicKey()))
}

func (p *Provider) Identity(_ *driver.IdentityOptions) (view.Identity, []byte, error) {
	sID, _, err := p.msp.Pseudonym()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed getting signing identity")
	}
	raw, err := sID.Serialize()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed serializing identity")
	}

	if p.signerService != nil {
		if err := p.signerService.RegisterSigner(context.Background(), raw, sID, sID); err != nil {
			return nil, nil, err
		}
	}

	return raw, nil, nil
}

func (p *Provider) EnrollmentID() string {
	return p.msp.EnrollmentID()
}

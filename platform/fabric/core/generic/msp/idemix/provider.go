/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/IBM/idemix"
	csp "github.com/IBM/idemix/bccsp"
	"github.com/IBM/idemix/bccsp/keystore"
	bccsp "github.com/IBM/idemix/bccsp/schemes"
	"github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.msp.idemix")

const (
	EIDIndex = 2
	RHIndex  = 3
)

const (
	Any bccsp.SignatureType = 100
)

// TODO: remove this
type SignerService interface {
	RegisterSigner(identity view.Identity, signer driver.Signer, verifier driver.Verifier) error
}

func GetSignerService(ctx view2.ServiceProvider) SignerService {
	s, err := ctx.GetService(reflect.TypeOf((*SignerService)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(SignerService)
}

type provider struct {
	*common
	userKey bccsp.Key
	conf    m.IdemixMSPConfig
	sp      view2.ServiceProvider

	sigType bccsp.SignatureType
	verType bccsp.VerificationType
}

func NewEIDNymProvider(conf1 *m.MSPConfig, sp view2.ServiceProvider) (*provider, error) {
	return NewProviderWithSigType(conf1, sp, bccsp.EidNym)
}

func NewStandardProvider(conf1 *m.MSPConfig, sp view2.ServiceProvider) (*provider, error) {
	return NewProviderWithSigType(conf1, sp, bccsp.Standard)
}

func NewAnyProvider(conf1 *m.MSPConfig, sp view2.ServiceProvider) (*provider, error) {
	return NewProviderWithSigType(conf1, sp, Any)
}

func NewProviderWithSigType(conf1 *m.MSPConfig, sp view2.ServiceProvider, sigType bccsp.SignatureType) (*provider, error) {
	logger.Debugf("Setting up Idemix-based MSP instance")

	if conf1 == nil {
		return nil, errors.Errorf("setup error: nil conf reference")
	}

	curve := math.Curves[math.FP256BN_AMCL]
	translator := &amcl.Fp256bn{C: curve}
	kvss := kvs.GetService(sp)
	keystore := &keystore.KVSStore{
		KVS:        kvss,
		Curve:      curve,
		Translator: translator,
	}

	cryptoProvider, err := csp.New(keystore, curve, translator, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting crypto provider")
	}

	var conf m.IdemixMSPConfig
	err = proto.Unmarshal(conf1.Config, &conf)
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling idemix provider config")
	}

	logger.Debugf("Setting up Idemix MSP instance %s", conf.Name)

	// Import Issuer Public Key
	issuerPublicKey, err := cryptoProvider.KeyImport(
		conf.Ipk,
		&bccsp.IdemixIssuerPublicKeyImportOpts{
			Temporary: true,
			AttributeNames: []string{
				idemix.AttributeNameOU,
				idemix.AttributeNameRole,
				idemix.AttributeNameEnrollmentId,
				idemix.AttributeNameRevocationHandle,
			},
		})
	if err != nil {
		return nil, err
	}

	// Import revocation public key
	RevocationPublicKey, err := cryptoProvider.KeyImport(
		conf.RevocationPk,
		&bccsp.IdemixRevocationPublicKeyImportOpts{Temporary: true},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to import revocation public key")
	}

	if conf.Signer == nil {
		// No credential in config, so we don't setup a default signer
		logger.Debug("idemix provider setup as verification only provider (no key material found)")
		return nil, errors.Errorf("idemix provider setup as verification only provider (no key material found)")
	}

	// A credential is present in the config, so we setup a default signer

	// Import User secret key
	userKey, err := cryptoProvider.KeyImport(conf.Signer.Sk, &bccsp.IdemixUserSecretKeyImportOpts{Temporary: true})
	if err != nil {
		return nil, errors.WithMessage(err, "failed importing signer secret key")
	}

	var verType bccsp.VerificationType
	switch sigType {
	case bccsp.Standard:
		verType = bccsp.ExpectStandard
	case bccsp.EidNym:
		verType = bccsp.ExpectEidNym
	case Any:
		verType = bccsp.BestEffort
	default:
		panic("invalid sig type")
	}
	if verType == bccsp.BestEffort {
		sigType = bccsp.Standard
	}

	return &provider{
		common: &common{
			name:            conf.Name,
			Csp:             cryptoProvider,
			IssuerPublicKey: issuerPublicKey,
			revocationPK:    RevocationPublicKey,
			epoch:           0,
			VerType:         verType,
		},
		userKey: userKey,
		conf:    conf,
		sp:      sp,
		sigType: sigType,
		verType: verType,
	}, nil
}

func (p *provider) Identity(opts *driver2.IdentityOptions) (view.Identity, []byte, error) {
	logger.Debug("getting new idemix identity")

	// Derive NymPublicKey
	nymKey, err := p.Csp.KeyDeriv(
		p.userKey,
		&bccsp.IdemixNymKeyDerivationOpts{
			Temporary: false,
			IssuerPK:  p.IssuerPublicKey,
		},
	)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed deriving nym")
	}
	NymPublicKey, err := nymKey.PublicKey()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed getting public nym key")
	}

	role := &m.MSPRole{
		MspIdentifier: p.name,
		Role:          m.MSPRole_MEMBER,
	}
	if checkRole(int(p.conf.Signer.Role), ADMIN) {
		role.Role = m.MSPRole_ADMIN
	}

	ou := &m.OrganizationUnit{
		MspIdentifier:                p.name,
		OrganizationalUnitIdentifier: p.conf.Signer.OrganizationalUnitIdentifier,
		CertifiersIdentifier:         p.IssuerPublicKey.SKI(),
	}

	enrollmentID := p.conf.Signer.EnrollmentId

	// Verify credential
	valid, err := p.Csp.Verify(
		p.userKey,
		p.conf.Signer.Cred,
		nil,
		&bccsp.IdemixCredentialSignerOpts{
			IssuerPK: p.IssuerPublicKey,
			Attributes: []bccsp.IdemixAttribute{
				{Type: bccsp.IdemixBytesAttribute, Value: []byte(p.conf.Signer.OrganizationalUnitIdentifier)},
				{Type: bccsp.IdemixIntAttribute, Value: getIdemixRoleFromMSPRole(role)},
				{Type: bccsp.IdemixBytesAttribute, Value: []byte(enrollmentID)},
				{Type: bccsp.IdemixHiddenAttribute},
			},
		},
	)
	if err != nil || !valid {
		return nil, nil, errors.WithMessage(err, "Credential is not cryptographically valid")
	}

	sigType := p.sigType
	var signerMetadata *bccsp.IdemixSignerMetadata
	if opts != nil {
		if opts.EIDExtension {
			sigType = bccsp.EidNym
		}
		if len(opts.AuditInfo) != 0 {
			ai, err := p.DeserializeAuditInfo(opts.AuditInfo)
			if err != nil {
				return nil, nil, err
			}
			signerMetadata = &bccsp.IdemixSignerMetadata{
				NymEIDAuditData: ai.NymEIDAuditData,
			}
		}
	}

	// Create the cryptographic evidence that this identity is valid
	sigOpts := &bccsp.IdemixSignerOpts{
		Credential: p.conf.Signer.Cred,
		Nym:        nymKey,
		IssuerPK:   p.IssuerPublicKey,
		Attributes: []bccsp.IdemixAttribute{
			{Type: bccsp.IdemixBytesAttribute},
			{Type: bccsp.IdemixIntAttribute},
			{Type: bccsp.IdemixHiddenAttribute},
			{Type: bccsp.IdemixHiddenAttribute},
		},
		RhIndex:  RHIndex,
		EidIndex: EIDIndex,
		CRI:      p.conf.Signer.CredentialRevocationInformation,
		SigType:  sigType,
		Metadata: signerMetadata,
	}
	proof, err := p.Csp.Sign(
		p.userKey,
		nil,
		sigOpts,
	)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "Failed to setup cryptographic proof of identity")
	}

	// Set up default signer
	sID := &signingIdentity{
		identity:     newIdentityWithVerType(p.common, NymPublicKey, role, ou, proof, p.verType),
		Cred:         p.conf.Signer.Cred,
		UserKey:      p.userKey,
		NymKey:       nymKey,
		enrollmentId: enrollmentID,
	}

	raw, err := sID.Serialize()
	if err != nil {
		return nil, nil, err
	}

	if err := GetSignerService(p.sp).RegisterSigner(raw, sID, sID); err != nil {
		return nil, nil, err
	}

	var infoRaw []byte
	switch sigType {
	case bccsp.Standard:
		infoRaw = nil
	case bccsp.EidNym:
		auditInfo := &AuditInfo{
			Csp:             p.Csp,
			IssuerPublicKey: p.IssuerPublicKey,
			NymEIDAuditData: sigOpts.Metadata.NymEIDAuditData,
			Attributes: [][]byte{
				[]byte(p.conf.Signer.OrganizationalUnitIdentifier),
				[]byte(strconv.Itoa(getIdemixRoleFromMSPRole(role))),
				[]byte(enrollmentID),
			},
		}
		infoRaw, err = auditInfo.Bytes()
		if err != nil {
			return nil, nil, err
		}
	default:
		panic("invalid sig type")
	}
	return raw, infoRaw, nil
}

func (p *provider) DeserializeVerifier(raw []byte) (driver.Verifier, error) {
	r, err := p.Deserialize(raw, true)
	if err != nil {
		return nil, err
	}

	return r.id, nil
}

func (p *provider) DeserializeSigner(raw []byte) (driver.Signer, error) {
	r, err := p.Deserialize(raw, true)
	if err != nil {
		return nil, err
	}

	nymKey, err := p.Csp.GetKey(r.NymPublicKey.SKI())
	if err != nil {
		return nil, errors.Wrap(err, "cannot find nym secret key")
	}

	si := &signingIdentity{
		identity:     r.id,
		Cred:         p.conf.Signer.Cred,
		UserKey:      p.userKey,
		NymKey:       nymKey,
		enrollmentId: p.conf.Signer.EnrollmentId,
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

func (p *provider) Info(raw []byte, auditInfo []byte) (string, error) {
	r, err := p.Deserialize(raw, true)
	if err != nil {
		return "", err
	}

	eid := ""
	if len(auditInfo) != 0 {
		ai := &AuditInfo{
			Csp:             p.Csp,
			IssuerPublicKey: p.IssuerPublicKey,
		}
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

func (p *provider) String() string {
	return fmt.Sprintf("Idemix Provider [%s]", hash.Hashable(p.Ipk).String())
}

func (p *provider) EnrollmentID() string {
	return p.conf.Signer.EnrollmentId
}

func (p *provider) DeserializeSigningIdentity(raw []byte) (driver.SigningIdentity, error) {
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
	NymPublicKey, err := p.Csp.KeyImport(
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

	id := newIdentityWithVerType(p.common, NymPublicKey, role, ou, serialized.Proof, p.verType)
	if err := id.Validate(); err != nil {
		return nil, errors.Wrap(err, "cannot deserialize, invalid identity")
	}
	nymKey, err := p.Csp.GetKey(NymPublicKey.SKI())
	if err != nil {
		return nil, errors.Wrap(err, "cannot find nym secret key")
	}

	return &signingIdentity{
		identity:     id,
		Cred:         p.conf.Signer.Cred,
		UserKey:      p.userKey,
		NymKey:       nymKey,
		enrollmentId: p.conf.Signer.EnrollmentId,
	}, nil
}

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
	ip "github.com/IBM/idemix/bccsp/schemes/dlog/crypto"
	"github.com/IBM/idemix/bccsp/schemes/dlog/crypto/translator/amcl"
	math "github.com/IBM/mathlib"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	m "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/pkg/errors"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.msp.idemix")

const (
	eidIndex = 2
	rhIndex  = 3
)

type deserialized struct {
	id           *identity
	NymPublicKey bccsp.Key
	si           *m.SerializedIdentity
	ou           *m.OrganizationUnit
	role         *m.MSPRole
}

type support struct {
	name            string
	ipk             []byte
	csp             bccsp.BCCSP
	issuerPublicKey bccsp.Key
	revocationPK    bccsp.Key
	epoch           int
}

func (s *support) Deserialize(raw []byte, checkValidity bool) (*deserialized, error) {
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
	NymPublicKey, err := s.csp.KeyImport(
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

	id := newIdentity(s, NymPublicKey, role, ou, serialized.Proof)
	if checkValidity {
		if err := id.Validate(); err != nil {
			return nil, errors.Wrap(err, "cannot deserialize, invalid identity")
		}
	}

	return &deserialized{
		id:           id,
		NymPublicKey: NymPublicKey,
		si:           si,
		ou:           ou,
		role:         role,
	}, nil
}

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
	*support
	userKey bccsp.Key
	conf    m.IdemixMSPConfig
	sp      view2.ServiceProvider
}

func NewProvider(conf1 *m.MSPConfig, sp view2.ServiceProvider) (*provider, error) {
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

	return &provider{
		support: &support{
			name:            conf.Name,
			csp:             cryptoProvider,
			issuerPublicKey: issuerPublicKey,
			revocationPK:    RevocationPublicKey,
			epoch:           0,
		},
		userKey: userKey,
		conf:    conf,
		sp:      sp,
	}, nil
}

func (p *provider) Identity() (view.Identity, []byte, error) {
	logger.Debug("getting new idemix identity")

	// Derive NymPublicKey
	nymKey, err := p.csp.KeyDeriv(
		p.userKey,
		&bccsp.IdemixNymKeyDerivationOpts{
			Temporary: false,
			IssuerPK:  p.issuerPublicKey,
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
		CertifiersIdentifier:         p.issuerPublicKey.SKI(),
	}

	enrollmentID := p.conf.Signer.EnrollmentId

	// Verify credential
	valid, err := p.csp.Verify(
		p.userKey,
		p.conf.Signer.Cred,
		nil,
		&bccsp.IdemixCredentialSignerOpts{
			IssuerPK: p.issuerPublicKey,
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

	// Create the cryptographic evidence that this identity is valid
	opts := &bccsp.IdemixSignerOpts{
		Credential: p.conf.Signer.Cred,
		Nym:        nymKey,
		IssuerPK:   p.issuerPublicKey,
		Attributes: []bccsp.IdemixAttribute{
			{Type: bccsp.IdemixBytesAttribute},
			{Type: bccsp.IdemixIntAttribute},
			{Type: bccsp.IdemixHiddenAttribute},
			{Type: bccsp.IdemixHiddenAttribute},
		},
		RhIndex:  rhIndex,
		EidIndex: eidIndex,
		CRI:      p.conf.Signer.CredentialRevocationInformation,
		SigType:  bccsp.EidNym,
	}
	proof, err := p.csp.Sign(
		p.userKey,
		nil,
		opts,
	)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "Failed to setup cryptographic proof of identity")
	}

	// Set up default signer
	sID := &signingIdentity{
		identity:     newIdentity(p.support, NymPublicKey, role, ou, proof),
		Cred:         p.conf.Signer.Cred,
		UserKey:      p.userKey,
		NymKey:       nymKey,
		enrollmentId: enrollmentID}

	raw, err := sID.Serialize()
	if err != nil {
		return nil, nil, err
	}

	err = GetSignerService(p.sp).RegisterSigner(raw, sID, sID)
	if err != nil {
		return nil, nil, err
	}

	ipk := new(ip.IssuerPublicKey)
	err = proto.Unmarshal(p.conf.Ipk, ipk)
	if err != nil {
		return nil, nil, err
	}

	auditInfo := &AuditInfo{
		NymEIDAuditData: opts.Metadata.NymEIDAuditData,
		Attributes: [][]byte{
			[]byte(p.conf.Signer.OrganizationalUnitIdentifier),
			[]byte(strconv.Itoa(getIdemixRoleFromMSPRole(role))),
			[]byte(enrollmentID),
		},
		IPK: ipk,
	}
	infoRaw, err := auditInfo.Bytes()
	if err != nil {
		return nil, nil, err
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

	nymKey, err := p.csp.GetKey(r.NymPublicKey.SKI())
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

func (p *provider) String() string {
	return fmt.Sprintf("Idemix Provider [%s]", hash.Hashable(p.ipk).String())
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
	NymPublicKey, err := p.csp.KeyImport(
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

	id := newIdentity(p.support, NymPublicKey, role, ou, serialized.Proof)
	if err := id.Validate(); err != nil {
		return nil, errors.Wrap(err, "cannot deserialize, invalid identity")
	}

	nymKey, err := p.csp.GetKey(NymPublicKey.SKI())
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

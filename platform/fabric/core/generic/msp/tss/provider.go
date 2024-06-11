/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tss

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tss "github.com/IBM/TSS/types"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/msp"
)

const (
	SignCerts = "signcerts"
)

type SignerService interface {
	RegisterSigner(identity view.Identity, signer fdriver.Signer, verifier fdriver.Verifier) error
}

type ContextFactory interface {
	InitiateContextWithIdentityAndID(view view2.View, id view.Identity, contextID string) (view.Context, error)
}

type IdentityProvider interface {
	Identity(label string) view.Identity
}

type Provider struct {
	enrollmentID string
	id           view.Identity
}

func NewProvider(cf ContextFactory, ip IdentityProvider, rootPath string, mspID string, signerService SignerService, opts MSPOpts) (*Provider, error) {
	storedData, err := os.ReadFile(filepath.Join(rootPath, "keystore", "priv_share"))
	if err != nil {
		return nil, fmt.Errorf("failed to load secret key share at [%s]: %w", rootPath, err)
	}

	certRaw, err := x509.LoadLocalMSPSignerCert(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate at [%s:%s]: %w", mspID, rootPath, err)
	}
	serRaw, err := x509.SerializeRaw(mspID, certRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to generate msp serailization at [%s:%s]: %w", mspID, rootPath, err)
	}
	cert, err := x509.PemDecodeCert(certRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate from [%s:%s]: %w", mspID, rootPath, err)
	}

	// in the case each org has exactly one peer
	// partyID is organization
	// universalID is peer
	mapping := map[tss.UniversalID]tss.PartyID{}
	nodes := strings.Split(strings.TrimSpace(opts.Nodes), ",")
	logger.Debugf("split [%s] in [%v]", opts.Nodes, nodes)
	uidToNode := map[tss.UniversalID]string{}
	for _, node := range nodes {
		logger.Debugf("split in attributes [%s]...", node)
		if len(node) == 0 {
			continue
		}
		nodeOpts := strings.Split(strings.TrimSpace(node), ":")
		if len(nodeOpts) != 3 {
			return nil, fmt.Errorf("invalid options, expects 3 elements, got [%d] from [%s]", len(nodeOpts), node)
		}
		// nodeOpts[0] = FSC Name
		// nodeOpts[1] = Universal ID
		uID, err := strconv.Atoi(nodeOpts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid options, cannot parse UID from [%s]", node)
		}
		// nodeOpts[2] = Party ID
		pID, err := strconv.Atoi(nodeOpts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid options, cannot parse PID from [%s]", node)
		}

		mapping[tss.UniversalID(uID)] = tss.PartyID(pID)
		uidToNode[tss.UniversalID(uID)] = nodeOpts[0]
	}
	mapping[opts.SelfID] = opts.PartyID
	uidToNode[opts.SelfID] = opts.ID
	logger.Debugf("tss mapping [%v]", mapping)
	logger.Debugf("uid to nodes [%v]", uidToNode)

	// register signer and verifier is required
	if signerService != nil {
		genericPublicKey, err := x509.PemDecodeKey(certRaw)
		if err != nil {
			return nil, fmt.Errorf("failed parsing received public key: %w", err)
		}
		publicKey, ok := genericPublicKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("expected *ecdsa.PublicKey")
		}

		contextSessionID := hash.Hashable(certRaw).String()
		thresholdSigner := NewSigner(cf, ip, opts, uidToNode, mapping, storedData, contextSessionID)
		err = signerService.RegisterSigner(
			serRaw,
			thresholdSigner,
			NewECDSAVerifier(publicKey),
		)
		if err != nil {
			return nil, fmt.Errorf("failed registering x509 signer: %w", err)
		}
	}

	return &Provider{
		enrollmentID: cert.Subject.CommonName,
		id:           serRaw,
	}, nil
}

func (p *Provider) Identity(opts *fdriver.IdentityOptions) (view.Identity, []byte, error) {
	return p.id, []byte(p.enrollmentID), nil
}

func (p *Provider) EnrollmentID() string {
	return p.enrollmentID
}

func (p *Provider) DeserializeVerifier(raw []byte) (fdriver.Verifier, error) {
	si := &msp.SerializedIdentity{}
	err := proto.Unmarshal(raw, si)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to msp.SerializedIdentity{}: %w", err)
	}
	genericPublicKey, err := x509.PemDecodeKey(si.IdBytes)
	if err != nil {
		return nil, fmt.Errorf("failed parsing received public key: %w", err)
	}
	publicKey, ok := genericPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("expected *ecdsa.PublicKey")
	}

	// TODO: check the validity of the identity against the msp

	return x509.NewVerifier(publicKey), nil
}

func (p *Provider) DeserializeSigner(raw []byte) (fdriver.Signer, error) {
	return nil, fmt.Errorf("not supported")
}

func (p *Provider) Info(raw []byte, auditInfo []byte) (string, error) {
	si := &msp.SerializedIdentity{}
	err := proto.Unmarshal(raw, si)
	if err != nil {
		return "", err
	}
	cert, err := x509.PemDecodeCert(si.IdBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MSP.x509: [%s][%s][%s]", view.Identity(raw).UniqueID(), si.Mspid, cert.Subject.CommonName), nil
}

func (p *Provider) String() string {
	return fmt.Sprintf("X509-TSS Provider for EID [%s]", p.enrollmentID)
}

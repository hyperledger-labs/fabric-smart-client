/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package idemix

import (
	"crypto/x509"
	"encoding/pem"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/common/tools/idemixgen/idemixca"
	"github.com/hyperledger/fabric/idemix"
	m "github.com/hyperledger/fabric/msp"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
)

func NewEphemeralSigningIdentity() ([]byte, api.SigningIdentity, error) {
	isk, ipkBytes, err := idemixca.GenerateIssuerKey()
	if err != nil {
		return nil, nil, err
	}
	revocationkey, err := idemix.GenerateLongTermRevocationKey()
	if err != nil {
		return nil, nil, err
	}
	ipk := &idemix.IssuerPublicKey{}
	err = proto.Unmarshal(ipkBytes, ipk)
	if err != nil {
		return nil, nil, err
	}
	key := &idemix.IssuerKey{Isk: isk, Ipk: ipk}
	conf, err := idemixca.GenerateSignerConfig(
		m.GetRoleMaskFromIdemixRole(m.MEMBER),
		"OU1", "enrollmentid1", 1, key, revocationkey)
	if err != nil {
		return nil, nil, err
	}

	encodedRevocationPK, err := x509.MarshalPKIXPublicKey(revocationkey.Public())
	if err != nil {
		return nil, nil, err
	}
	pemEncodedRevocationPK := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: encodedRevocationPK})

	// prepare msp config
	idemixConfig := &msp.IdemixMSPConfig{
		Name:         "ephemeral",
		Ipk:          ipkBytes,
		RevocationPk: pemEncodedRevocationPK,
	}
	signerConfig := &msp.IdemixMSPSignerConfig{}
	err = proto.Unmarshal(conf, signerConfig)
	if err != nil {
		return nil, nil, err
	}
	idemixConfig.Signer = signerConfig

	confBytes, err := proto.Marshal(idemixConfig)
	if err != nil {
		return nil, nil, err
	}

	provider, err := NewProvider(
		&msp.MSPConfig{Config: confBytes, Type: int32(m.IDEMIX)},
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	si, err := provider.SignerIdentity()
	if err != nil {
		return nil, nil, err
	}

	return ipkBytes, si, nil
}

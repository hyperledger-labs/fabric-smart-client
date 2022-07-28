/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	x5092 "crypto/x509"
	"encoding/pem"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	msp2 "github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
)

// Identity refers to the creator of a tx;
type Identity interface {
	Serialize() ([]byte, error)

	Verify(msg []byte, sig []byte) error
}

// SigningIdentity defines the functions necessary to sign an
// array of bytes; it is needed to sign the commands transmitted to
// the prover peer service.
type SigningIdentity interface {
	Identity //extends Identity

	Sign(msg []byte) ([]byte, error)
}

// GetSigningIdentity retrieves a signing identity from the passed arguments
func GetSigningIdentity(mspConfigPath, mspID string) (SigningIdentity, error) {
	mspInstance, err := LoadLocalMSPAt(mspConfigPath, mspID, "bccsp")
	if err != nil {
		return nil, err
	}

	signingIdentity, err := mspInstance.GetDefaultSigningIdentity()
	if err != nil {
		return nil, err
	}

	return signingIdentity, nil
}

// LoadLocalMSPAt loads an MSP whose configuration is stored at 'dir', and whose
// id and type are the passed as arguments.
func LoadLocalMSPAt(dir, id, mspType string) (msp.MSP, error) {
	if mspType != "bccsp" {
		return nil, errors.Errorf("invalid msp type, expected 'bccsp', got %s", mspType)
	}
	conf, err := msp.GetLocalMspConfig(dir, nil, id)
	if err != nil {
		return nil, err
	}
	ks, err := sw.NewFileBasedKeyStore(nil, filepath.Join(dir, "keystore"), true)
	if err != nil {
		return nil, err
	}
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	if err != nil {
		return nil, err
	}
	thisMSP, err := msp.NewBccspMspWithKeyStore(msp.MSPv1_0, ks, cryptoProvider)
	if err != nil {
		return nil, err
	}
	err = thisMSP.Setup(conf)
	if err != nil {
		return nil, err
	}
	return thisMSP, nil
}

func GetEnrollmentID(id []byte) (string, error) {
	si := &msp2.SerializedIdentity{}
	err := proto.Unmarshal(id, si)
	if err != nil {
		return "", errors.Wrap(err, "failed to unmarshal to msp.SerializedIdentity{}")
	}
	block, _ := pem.Decode(si.IdBytes)
	if block == nil {
		return "", errors.New("bytes are not PEM encoded")
	}
	switch block.Type {
	case "CERTIFICATE":
		cert, err := x5092.ParseCertificate(block.Bytes)
		if err != nil {
			return "", errors.WithMessage(err, "pem bytes are not cert encoded ")
		}
		return cert.Subject.CommonName, nil
	default:
		return "", errors.Errorf("bad block type %s, expected CERTIFICATE", block.Type)
	}
}

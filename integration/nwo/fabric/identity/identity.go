/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package identity

import (
	"path/filepath"

	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
)

//go:generate counterfeiter -o client/mock/identity.go -fake-name Identity . Identity

// Identity refers to the creator of a tx;
type Identity interface {
	Serialize() ([]byte, error)
}

//go:generate counterfeiter -o client/mock/signing_identity.go -fake-name SigningIdentity . SigningIdentity

// SigningIdentity defines the functions necessary to sign an
// array of bytes; it is needed to sign the commands transmitted to
// the prover peer service.
type SigningIdentity interface {
	Identity // extends Identity

	Sign(msg []byte) ([]byte, error)
}

// GetSigningIdentity retrieves a signing identity from the passed arguments
func GetSigningIdentity(mspConfigPath, mspID, mspType string) (SigningIdentity, error) {
	mspInstance, err := LoadLocalMSPAt(mspConfigPath, mspID, mspType)
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

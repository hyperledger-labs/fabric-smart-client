/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package file

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/ecdsa"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   "file",
		Driver: &Driver{},
	}
}

type Driver struct{}

func (d *Driver) Load(configProvider driver.ConfigProvider) (view.Identity, driver.Signer, driver.Verifier, error) {
	idPEM, err := os.ReadFile(configProvider.GetPath("fsc.identity.cert.file"))
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed loading SFC Node Identity")
	}
	fileCont, err := os.ReadFile(configProvider.GetPath("fsc.identity.key.file"))
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed reading file [%s]", fileCont)
	}
	id, verifier, err := ecdsa.NewIdentityFromPEMCert(idPEM)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed loading default verifier")
	}
	signer, err := ecdsa.NewSignerFromPEM(fileCont)
	if err != nil {
		return id, nil, verifier, errors.Wrapf(err, "failed loading default signer")
	}
	return id, signer, verifier, nil
}

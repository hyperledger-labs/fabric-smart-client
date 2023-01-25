/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"io/ioutil"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/ecdsa"
	kms "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

	"github.com/pkg/errors"
)

type Driver struct{}

func (d *Driver) Load(configProvider kms.ConfigProvider, pemBytes []byte) (view.Identity, driver.Signer, driver.Verifier, error) {
	fileCont, err := ioutil.ReadFile(configProvider.GetPath("fsc.identity.key.file"))
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed reading file [%s]", fileCont)
	}
	id, verifier, err := ecdsa.NewIdentityFromPEMCert(pemBytes)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed loading default verifier")
	}
	signer, err := ecdsa.NewSignerFromPEM(fileCont)
	if err != nil {
		return id, nil, verifier, errors.Wrapf(err, "failed loading default signer")
	}
	return id, signer, verifier, nil
}

func init() {
	kms.Register("default", &Driver{})
}

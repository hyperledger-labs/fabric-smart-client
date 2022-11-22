/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"io/ioutil"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

const (
	MSPType = "bccsp"
)

var logger = flogging.MustGetLogger("fabric-sdk.msp.x509")

type IdentityLoader struct{}

func (i *IdentityLoader) Load(manager driver.Manager, c config.MSP) error {
	// Try without "msp"
	var bccspOpts *config.BCCSP
	if c.Opts != nil {
		bccspOpts = c.Opts.BCCSP
	}
	provider, err := NewProviderWithBCCSPConfig(
		manager.Config().TranslatePath(c.Path),
		c.MSPID,
		manager.SignerService(),
		bccspOpts,
	)
	if err != nil {
		logger.Warnf("failed reading bccsp msp configuration from [%s]: [%s]", filepath.Join(manager.Config().TranslatePath(c.Path), c.ID), err)
		// Try with "msp"
		provider, err = NewProviderWithBCCSPConfig(
			filepath.Join(manager.Config().TranslatePath(c.Path), "msp"),
			c.MSPID,
			manager.SignerService(),
			bccspOpts,
		)
		if err != nil {
			logger.Warnf("failed reading bccsp msp configuration from [%s and %s]: [%s]",
				filepath.Join(manager.Config().TranslatePath(c.Path),
					filepath.Join(manager.Config().TranslatePath(c.Path), "msp")), err,
			)
			return errors.WithMessagef(err, "failed to load BCCSP MSP configuration [%s]", c.ID)
		}
	}

	manager.AddDeserializer(provider)
	manager.AddMSP(c.ID, c.MSPType, provider.EnrollmentID(), provider.Identity)

	// set default
	defaultIdentity, _, err := provider.Identity(nil)
	if err != nil {
		return errors.WithMessagef(err, "failed to get default identity for [%s]", c.MSPID)
	}
	defaultSigningIdentity, err := provider.SerializedIdentity()
	if err != nil {
		return errors.WithMessagef(err, "failed to get default signing identity for [%s]", c.MSPID)
	}
	manager.SetDefaultIdentity(c.ID, defaultIdentity, defaultSigningIdentity)

	return nil
}

type FolderIdentityLoader struct {
	*IdentityLoader
}

func (f *FolderIdentityLoader) Load(manager driver.Manager, c config.MSP) error {
	entries, err := ioutil.ReadDir(manager.Config().TranslatePath(c.Path))
	if err != nil {
		logger.Warnf("failed reading from [%s]: [%s]", manager.Config().TranslatePath(c.Path), err)
		return errors.Wrapf(err, "failed reading from [%s]", manager.Config().TranslatePath(c.Path))
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()

		if err := f.IdentityLoader.Load(manager, config.MSP{
			ID:      id,
			MSPType: MSPType,
			MSPID:   id,
			Path:    filepath.Join(manager.Config().TranslatePath(c.Path), id),
			Opts:    c.Opts,
		}); err != nil {
			return errors.WithMessagef(err, "failed to load BCCSP MSP configuration [%s]", id)
		}
	}
	return nil
}

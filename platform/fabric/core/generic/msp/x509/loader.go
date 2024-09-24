/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const (
	MSPType       = "bccsp"
	BCCSPOptField = "bccsp" // viper converts map keys to lowercase
)

var logger = flogging.MustGetLogger("fabric-sdk.msp.x509")

type IdentityLoader struct{}

func (i *IdentityLoader) Load(manager driver.Manager, c config.MSP) error {
	var bccspOpts *config.BCCSP
	if c.Opts != nil {
		logger.Debugf("Options [%v]", c.Opts)
		bccspOptsBoxed, ok := c.Opts[BCCSPOptField]
		if ok {
			var err error
			bccspOpts, err = ToBCCSPOpts(bccspOptsBoxed)
			if err != nil {
				return errors.Wrapf(err, "failed to unmarshal BCCSP opts")
			}
			logger.Debugf("Options unmarshalled [%v]", bccspOpts)
		}
	}

	// Try without "msp"
	rootPath := filepath.Join(manager.Config().TranslatePath(c.Path))
	provider, err := NewProviderWithBCCSPConfig(rootPath, "", c.MSPID, manager.SignerService(), bccspOpts)
	if err != nil {
		logger.Warnf("failed reading bccsp msp configuration from [%s]: [%s]", rootPath, err)
		// Try with "msp"
		provider, err = NewProviderWithBCCSPConfig(filepath.Join(rootPath, "msp"), "", c.MSPID, manager.SignerService(), bccspOpts)
		if err != nil {
			logger.Warnf("failed reading bccsp msp configuration from [%s and %s]: [%s]",
				rootPath, filepath.Join(rootPath, "msp"), err,
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
	entries, err := os.ReadDir(manager.Config().TranslatePath(c.Path))
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

func ToBCCSPOpts(boxed interface{}) (*config.BCCSP, error) {
	opts := &config.BCCSP{}
	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true, // allow pin to be a string
		Result:           &opts,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return opts, err
	}

	err = decoder.Decode(boxed)
	return opts, err
}

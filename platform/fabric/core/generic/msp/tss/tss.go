/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tss

import (
	"fmt"
	"path/filepath"

	tss "github.com/IBM/TSS/types"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"gopkg.in/yaml.v2"
)

const (
	MSPType  = "bccsp-tss"
	OptField = "tss"
)

var logger = flogging.MustGetLogger("fabric-sdk.msp.tss")

type MSPOpts struct {
	ID        string          `yaml:"id,omitempty"`
	Threshold int             `yaml:"threshold,omitempty"`
	SelfID    tss.UniversalID `yaml:"self-id,omitempty"`
	PartyID   tss.PartyID     `yaml:"party-id,omitempty"`
	Nodes     string          `yaml:"nodes,omitempty"`
}

type IdentityLoader struct {
	sp view.ServiceProvider
}

func (i *IdentityLoader) Load(manager driver.Manager, c config.MSP) error {
	logger.Debugf("loading BCCSP-TSS identity [%v]...", c)

	var opts MSPOpts
	if c.Opts != nil {
		logger.Debugf("Options [%v]", c.Opts)
		boxed, ok := c.Opts[OptField]
		if ok {
			tmp, err := ToMSPOpts(boxed)
			if err != nil {
				return fmt.Errorf("failed to unmarshal BCCSP-TSS opts: %w", err)
			}
			opts = *tmp
			logger.Debugf("Options unmarshalled [%v]", opts)
		}
	} else {
		return fmt.Errorf("missing options for MSP identity [%s][%v]", c.ID, c.Opts)
	}

	// Try without "msp"
	rootPath := manager.Config().TranslatePath(c.Path)
	provider, err := NewProvider(
		view.GetManager(i.sp),
		view.GetIdentityProvider(i.sp),
		rootPath,
		c.MSPID,
		manager.SignerService(),
		opts,
	)
	if err != nil {
		logger.Warnf("failed reading bccsp-tss msp configuration from [%s]: [%s]", rootPath, err)
		// Try with "msp"
		provider, err = NewProvider(
			view.GetManager(i.sp),
			view.GetIdentityProvider(i.sp),
			filepath.Join(rootPath, "msp"),
			c.MSPID,
			manager.SignerService(),
			opts,
		)
		if err != nil {
			logger.Warnf("failed reading bccsp-tss msp configuration from [%s and %s]: [%s]",
				filepath.Join(rootPath), filepath.Join(rootPath, "msp"), err,
			)
			return fmt.Errorf("failed to load BCCSP-TSS MSP configuration [%s]: %w", c.ID, err)
		}
	}

	manager.AddDeserializer(provider)
	manager.AddMSP(c.ID, c.MSPType, provider.EnrollmentID(), provider.Identity)

	return nil
}

func ToMSPOpts(boxed interface{}) (*MSPOpts, error) {
	raw, err := yaml.Marshal(boxed)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %w", err)
	}
	logger.Debugf("marshal [%v] to [%s]", boxed, string(raw))
	opts := &MSPOpts{}
	if err := yaml.Unmarshal(raw, opts); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}
	logger.Debugf("unmarshal back [%s] to [%v]", string(raw), opts)
	return opts, nil
}

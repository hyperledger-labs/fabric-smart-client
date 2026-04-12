/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"os"
	"path/filepath"
	"strings"

	math "github.com/IBM/mathlib"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
)

const (
	MSPType = "idemix"
)

type IdentityLoader struct {
	KVS           KVS
	SignerService driver.SignerService
}

func (i *IdentityLoader) Load(manager driver.Manager, c config.MSP) error {
	conf, err := fabricmsp.GetLocalMspConfigWithType(manager.Config().TranslatePath(c.Path), nil, c.MSPID, c.MSPType)
	if err != nil {
		return errors.Wrapf(err, "failed reading idemix msp configuration from [%s]", manager.Config().TranslatePath(c.Path))
	}

	curveID, err := ParseCurveID(c.CurveID)
	if err != nil {
		return errors.Wrapf(err, "invalid curve ID [%s] for idemix msp [%s]", c.CurveID, c.ID)
	}

	provider, err := NewProviderWithAnyPolicyAndCurve(conf, i.KVS, i.SignerService, curveID)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", manager.Config().TranslatePath(c.Path))
	}
	manager.AddDeserializer(provider)
	cacheSize := manager.CacheSize()
	if c.CacheSize > 0 {
		cacheSize = c.CacheSize
	}
	if err := manager.AddMSP(c.ID, c.MSPType, provider.EnrollmentID(), NewIdentityCache(provider.Identity, cacheSize, nil).Identity); err != nil {
		return errors.Wrapf(err, "failed adding idemix msp [%s]", manager.Config().TranslatePath(c.Path))
	}
	logger.Debugf("added %s msp for id %s with curve %d and cache of size %d", c.MSPType, c.ID+"@"+provider.EnrollmentID(), curveID, cacheSize)

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
			CurveID: c.CurveID,
		}); err != nil {
			return errors.WithMessagef(err, "failed to load Idemix MSP configuration [%s]", id)
		}
	}
	return nil
}

// ParseCurveID maps a curve name string to its math.CurveID constant.
// If name is empty, it defaults to math.FP256BN_AMCL for backward compatibility.
func ParseCurveID(name string) (math.CurveID, error) {
	if name == "" {
		return math.FP256BN_AMCL, nil
	}

	switch strings.ToUpper(name) {
	case "BN254":
		return math.BN254, nil
	case "FP256BN_AMCL":
		return math.FP256BN_AMCL, nil
	case "FP256BN_AMCL_MIRACL":
		return math.FP256BN_AMCL_MIRACL, nil
	case "BLS12_377_GURVY":
		return math.BLS12_377_GURVY, nil
	case "BLS12_381_BBS":
		return math.BLS12_381_BBS, nil
	default:
		return 0, errors.Errorf("unsupported idemix curve: %s", name)
	}
}

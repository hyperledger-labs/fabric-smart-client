/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package idemix

import (
	"os"
	"path/filepath"

	idemixconfig "github.com/IBM/idemix/msp/config"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
)

const (
	MSPType = "idemix"
)

type IdentityLoader struct {
	KVS           KVS
	SignerService driver.SignerService
}

// stampCurveID overwrites curveID on conf's embedded IdemixMSPConfig, so that Setup
// resolves the curve/translator pair explicitly named by curveID rather than falling back
// to its scheme's default curve. GetLocalMspConfigWithType never populates CurveId itself:
// idemixgen does not persist the curve name in any of the on-disk MSP fixture files.
func stampCurveID(conf *m.MSPConfig, curveID string) error {
	var idemixCfg idemixconfig.IdemixMSPConfig
	if err := proto.Unmarshal(conf.Config, &idemixCfg); err != nil {
		return errors.Wrap(err, "failed unmarshalling idemix msp config")
	}
	idemixCfg.CurveId = curveID
	raw, err := proto.Marshal(&idemixCfg)
	if err != nil {
		return errors.Wrap(err, "failed marshalling idemix msp config")
	}
	conf.Config = raw
	return nil
}

func (i *IdentityLoader) Load(manager driver.Manager, c config.MSP) error {
	conf, err := fabricmsp.GetLocalMspConfigWithType(manager.Config().TranslatePath(c.Path), nil, c.MSPID, c.MSPType)
	if err != nil {
		return errors.Wrapf(err, "failed reading idemix msp configuration from [%s]", manager.Config().TranslatePath(c.Path))
	}
	if c.CurveID != "" {
		if err := stampCurveID(conf, c.CurveID); err != nil {
			return errors.Wrapf(err, "failed setting curve id [%s] on idemix msp configuration from [%s]", c.CurveID, manager.Config().TranslatePath(c.Path))
		}
	}
	provider, err := NewProvider(conf, i.KVS, i.SignerService)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", manager.Config().TranslatePath(c.Path))
	}
	manager.AddDeserializer(provider)
	cacheSize := manager.CacheSize()
	if c.CacheSize > 0 {
		cacheSize = c.CacheSize
	}
	if err := manager.AddMSP(
		c.ID,
		c.MSPType,
		provider.EnrollmentID(),
		NewIdentityCache(provider.Identity, cacheSize, nil).Identity,
	); err != nil {
		return errors.Wrapf(err, "failed adding idemix msp [%s]", manager.Config().TranslatePath(c.Path))
	}
	logger.Debugf("added %s msp for id %s with cache of size %d", c.MSPType, c.ID+"@"+provider.EnrollmentID(), cacheSize)

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
		}); err != nil {
			return errors.WithMessagef(err, "failed to load Idemix MSP configuration [%s]", id)
		}
	}
	return nil
}

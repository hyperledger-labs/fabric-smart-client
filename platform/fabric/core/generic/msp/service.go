/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
)

const (
	IdemixMSP       = "idemix"
	IdemixMSPFolder = "idemix-folder"
	BccspMSP        = "bccsp"
	BccspMSPFolder  = "bccsp-folder"
)

var logger = logging.MustGetLogger("fabric-sdk.msp")

type KVS interface {
	Exists(id string) bool
	Put(id string, state interface{}) error
	Get(id string, state interface{}) error
}

type service struct {
	defaultIdentity        view.Identity
	defaultSigningIdentity driver.SigningIdentity
	signerService          driver.SignerService
	binderService          driver.BinderService
	deserializerManager    driver.DeserializerManager
	defaultViewIdentity    view.Identity
	KVS                    KVS
	config                 driver.Config

	mspsMutex           sync.RWMutex
	defaultMSP          string
	identityLoaders     map[string]driver.IdentityLoader
	msps                []*driver.MSP
	mspsByName          map[string]*driver.MSP
	mspsByEnrollmentID  map[string]*driver.MSP
	mspsByTypeAndName   map[string]*driver.MSP
	bccspMspsByIdentity map[string]*driver.MSP
	cacheSize           int
}

func NewLocalMSPManager(
	config driver.Config,
	KVS KVS,
	signerService driver.SignerService,
	binderService driver.BinderService,
	defaultViewIdentity view.Identity,
	deserializerManager driver.DeserializerManager,
	cacheSize int,
) *service {
	s := &service{
		config:              config,
		KVS:                 KVS,
		signerService:       signerService,
		binderService:       binderService,
		deserializerManager: deserializerManager,
		defaultViewIdentity: defaultViewIdentity,
		mspsByTypeAndName:   map[string]*driver.MSP{},
		bccspMspsByIdentity: map[string]*driver.MSP{},
		mspsByEnrollmentID:  map[string]*driver.MSP{},
		mspsByName:          map[string]*driver.MSP{},
		cacheSize:           cacheSize,
		identityLoaders:     map[string]driver.IdentityLoader{},
	}
	s.PutIdentityLoader(BccspMSP, &x509.IdentityLoader{})
	s.PutIdentityLoader(BccspMSPFolder, &x509.FolderIdentityLoader{})
	s.PutIdentityLoader(IdemixMSP, &idemix.IdentityLoader{
		KVS:           KVS,
		SignerService: signerService,
	})
	s.PutIdentityLoader(IdemixMSPFolder, &idemix.FolderIdentityLoader{
		IdentityLoader: &idemix.IdentityLoader{
			KVS:           KVS,
			SignerService: signerService,
		},
	})
	return s
}

func (s *service) AddDeserializer(deserializer driver.Deserializer) {
	s.deserializerManager.AddDeserializer(deserializer)
}

func (s *service) Config() driver.Config {
	return s.config
}

func (s *service) DefaultMSP() string {
	return s.config.DefaultMSP()
}

func (s *service) SignerService() driver.SignerService {
	return s.signerService
}

func (s *service) CacheSize() int {
	return s.cacheSize
}

func (s *service) SetDefaultIdentity(id string, defaultIdentity view.Identity, defaultSigningIdentity driver.SigningIdentity) {
	if id == s.defaultMSP {
		if s.defaultIdentity == nil {
			logger.Infof("setting default identity to [%s]", id)
		}

		// set default
		s.defaultIdentity = defaultIdentity
		s.defaultSigningIdentity = defaultSigningIdentity
	}
}

func (s *service) DefaultIdentity() view.Identity {
	return s.defaultIdentity
}

func (s *service) AnonymousIdentity() (view.Identity, error) {
	id, err := s.Identity("idemix")
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get default anonymous identity labelled `idemix`")
	}
	if err := s.binderService.Bind(s.defaultViewIdentity, id); err != nil {
		return nil, errors.WithMessagef(err, "failed to bind identity [%s] to default [%s]", id, s.defaultViewIdentity)
	}
	return id, nil
}

func (s *service) Identity(label string) (view.Identity, error) {
	id, err := s.GetIdentityByID(label)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get identity [%s]", label)
	}
	return id, nil
}

func (s *service) IsMe(id view.Identity) bool {
	return s.signerService.IsMe(id)
}

func (s *service) DefaultSigningIdentity() fdriver.SigningIdentity {
	return s.defaultSigningIdentity
}

func (s *service) GetIdentityInfoByLabel(mspType string, label string) *fdriver.IdentityInfo {
	s.mspsMutex.RLock()
	defer s.mspsMutex.RUnlock()

	logger.Debugf("get identity info by label [%s:%s]", mspType, label)
	r, ok := s.mspsByTypeAndName[mspType+label]
	if !ok {
		logger.Debugf("identity info not found for label [%s:%s][%v]", mspType, label, s.mspsByTypeAndName)
		return nil
	}
	return &fdriver.IdentityInfo{
		ID:           r.Name,
		EnrollmentID: r.EnrollmentID,
		GetIdentity:  r.GetIdentity,
	}
}

func (s *service) GetIdentityInfoByIdentity(mspType string, id view.Identity) *fdriver.IdentityInfo {
	s.mspsMutex.RLock()
	defer s.mspsMutex.RUnlock()

	if mspType == BccspMSP {
		r, ok := s.bccspMspsByIdentity[id.String()]
		if !ok {
			return nil
		}
		return &fdriver.IdentityInfo{
			ID:           r.Name,
			EnrollmentID: r.EnrollmentID,
			GetIdentity:  r.GetIdentity,
		}
	}

	// scan all msps in the worst case
	for _, r := range s.msps {
		if r.Type == mspType {
			lid, _, err := r.GetIdentity(nil)
			if err != nil {
				continue
			}
			if id.Equal(lid) {
				return &fdriver.IdentityInfo{
					ID:           r.Name,
					EnrollmentID: r.EnrollmentID,
					GetIdentity:  r.GetIdentity,
				}
			}
		}
	}
	return nil
}

func (s *service) GetIdentityByID(id string) (view.Identity, error) {
	s.mspsMutex.RLock()
	defer s.mspsMutex.RUnlock()

	// Check indices first
	r, ok := s.mspsByName[id]
	if ok {
		identity, _, err := r.GetIdentity(nil)
		return identity, err
	}

	r, ok = s.mspsByEnrollmentID[id]
	if ok {
		identity, _, err := r.GetIdentity(nil)
		return identity, err
	}

	// Scan
	for _, r := range s.msps {
		if r.Name == id || r.EnrollmentID == id {
			identity, _, err := r.GetIdentity(nil)
			return identity, err
		}
	}

	identity, err := s.binderService.GetIdentity(id, nil)
	if err != nil {
		return nil, errors.Errorf("identity [%s] not found", id)
	}
	return identity, nil
}

func (s *service) RegisterIdemixMSP(id string, path string, mspID string) error {
	s.mspsMutex.Lock()
	defer s.mspsMutex.Unlock()

	conf, err := msp.GetLocalMspConfigWithType(path, nil, mspID, IdemixMSP)
	if err != nil {
		return errors.Wrapf(err, "failed reading idemix msp configuration from [%s]", path)
	}
	provider, err := idemix.NewProviderWithAnyPolicy(conf, s.KVS, s.signerService)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", path)
	}

	s.deserializerManager.AddDeserializer(provider)
	if err := s.AddMSP(id, IdemixMSP, provider.EnrollmentID(), idemix.NewIdentityCache(provider.Identity, s.cacheSize, nil).Identity); err != nil {
		return errors.Wrapf(err, "failed adding idemix msp [%s] to [%s]", id, path)
	}
	logger.Debugf("added IdemixMSP msp for id %s with cache of size %d", id+"@"+provider.EnrollmentID(), s.cacheSize)
	return nil
}

func (s *service) RegisterX509MSP(id string, path string, mspID string) error {
	s.mspsMutex.Lock()
	defer s.mspsMutex.Unlock()

	provider, err := x509.NewProvider(path, "", mspID, s.signerService)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", path)
	}

	s.deserializerManager.AddDeserializer(provider)
	if err := s.AddMSP(id, BccspMSP, provider.EnrollmentID(), provider.Identity); err != nil {
		return errors.Wrapf(err, "failed adding bccsp msp [%s] to [%s]", id, path)
	}

	return nil
}

func (s *service) Refresh() error {
	s.mspsMutex.Lock()
	defer s.mspsMutex.Unlock()

	// clean cashes
	s.msps = nil
	s.mspsByTypeAndName = map[string]*driver.MSP{}
	s.bccspMspsByIdentity = map[string]*driver.MSP{}
	s.mspsByEnrollmentID = map[string]*driver.MSP{}
	s.mspsByName = map[string]*driver.MSP{}

	// reload
	if err := s.loadLocalMSPs(); err != nil {
		return err
	}

	return nil
}

func (s *service) AddMSP(name string, mspType string, enrollmentID string, IdentityGetter fdriver.GetIdentityFunc) error {
	if mspType == BccspMSP && s.binderService != nil {
		id, _, err := IdentityGetter(nil)
		if err != nil {
			return errors.Wrapf(err, "cannot get identity for [%s,%s,%s][%s]", name, mspType, enrollmentID, err)
		}
		if err := s.binderService.Bind(s.defaultViewIdentity, id); err != nil {
			return errors.Wrapf(err, "cannot bind identity for [%s,%s,%s][%s]", name, mspType, enrollmentID, err)
		}
	}

	msp := &driver.MSP{
		Name:         name,
		Type:         mspType,
		EnrollmentID: enrollmentID,
		GetIdentity:  IdentityGetter,
	}
	if mspType == BccspMSP {
		id, _, err := IdentityGetter(nil)
		if err != nil {
			return errors.Wrapf(err, "cannot get identity for [%s,%s,%s][%s]", name, mspType, enrollmentID, err)
		}
		s.bccspMspsByIdentity[id.String()] = msp
		logger.Debugf("add bccsp msp for id %s, identity [%s]", name+"@"+enrollmentID, id.String())
	} else {
		logger.Debugf("add idemix msp for id %s", name+"@"+enrollmentID)
	}
	s.mspsByTypeAndName[mspType+name] = msp
	s.mspsByName[name] = msp
	if len(enrollmentID) != 0 {
		s.mspsByEnrollmentID[enrollmentID] = msp
	}
	s.msps = append(s.msps, msp)
	return nil
}

func (s *service) PutIdentityLoader(idType string, loader driver.IdentityLoader) {
	s.mspsMutex.Lock()
	defer s.mspsMutex.Unlock()

	s.identityLoaders[idType] = loader
}

func (s *service) Load() error {
	s.mspsMutex.Lock()
	defer s.mspsMutex.Unlock()

	if err := s.loadLocalMSPs(); err != nil {
		return err
	}
	return nil
}

func (s *service) Msps() []string {
	s.mspsMutex.RLock()
	defer s.mspsMutex.RUnlock()

	var res []string
	for _, r := range s.msps {
		res = append(res, r.Name)
	}
	return res
}

func (s *service) loadLocalMSPs() error {
	configs, err := s.config.MSPs()
	if err != nil {
		return errors.WithMessagef(err, "failed loading local MSP configs")
	}
	s.defaultMSP = s.config.DefaultMSP()
	if len(s.defaultMSP) == 0 {
		if len(configs) == 0 {
			return errors.New("default MSP not configured and no MSPs set")
		}
		logger.Warnf("default MSP not configured, set it to [%s]", configs[0].ID)
		s.defaultMSP = configs[0].ID
	}

	logger.Debugf("Local Local [%d] MSPS using default [%s]", len(configs), s.defaultMSP)
	for _, config := range configs {
		loader, ok := s.identityLoaders[config.MSPType]
		if !ok {
			logger.Warnf("msp type [%s] not recognized, skipping", config.MSPType)
			continue
		}
		if err := loader.Load(s, config); err != nil {
			return errors.WithMessagef(err, "failed to load msp [%s:%s] at [%s]", config.ID, config.MSPType, config.Path)
		}
	}

	if s.defaultIdentity == nil {
		return errors.Errorf("no default identity set for network [%s]", s.config.NetworkName())
	}

	return nil
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
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

var logger = flogging.MustGetLogger("fabric-sdk.msp")

type service struct {
	sp                     view2.ServiceProvider
	defaultIdentity        view.Identity
	defaultSigningIdentity driver.SigningIdentity
	signerService          driver.SignerService
	binderService          driver.BinderService
	defaultViewIdentity    view.Identity
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
	sp view2.ServiceProvider,
	config driver.Config,
	signerService driver.SignerService,
	binderService driver.BinderService,
	defaultViewIdentity view.Identity,
	cacheSize int,
) *service {
	s := &service{
		sp:                  sp,
		config:              config,
		signerService:       signerService,
		binderService:       binderService,
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
	s.PutIdentityLoader(IdemixMSP, &idemix.IdentityLoader{})
	s.PutIdentityLoader(IdemixMSPFolder, &idemix.FolderIdentityLoader{})
	return s
}

func (s *service) AddDeserializer(deserializer sig.Deserializer) {
	s.DeserializerManager().AddDeserializer(deserializer)
}

func (s *service) Config() driver.Config {
	return s.config
}

func (s *service) DefaultMSP() string {
	//TODO implement me
	panic("implement me")
}

func (s *service) SignerService() driver.SignerService {
	return s.signerService
}

func (s *service) ServiceProvider() view2.ServiceProvider {
	return s.sp
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

func (s *service) AnonymousIdentity() view.Identity {
	id := s.Identity("idemix")

	es := view2.GetEndpointService(s.sp)
	if err := es.Bind(s.defaultViewIdentity, id); err != nil {
		panic(err)
	}

	return id
}

func (s *service) Identity(label string) view.Identity {
	id, err := s.GetIdentityByID(label)
	if err != nil {
		panic(err)
	}
	return id
}

func (s *service) IsMe(id view.Identity) bool {
	return view2.GetSigService(s.sp).IsMe(id)
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

	identity, err := view2.GetEndpointService(s.sp).GetIdentity(id, nil)
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
	provider, err := idemix.NewAnyProvider(conf, s.sp)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", path)
	}

	s.DeserializerManager().AddDeserializer(provider)
	s.AddMSP(id, IdemixMSP, provider.EnrollmentID(), idemix.NewIdentityCache(provider.Identity, s.cacheSize).Identity)
	logger.Debugf("added IdemixMSP msp for id %s with cache of size %d", id+"@"+provider.EnrollmentID(), s.cacheSize)
	return nil
}

func (s *service) RegisterX509MSP(id string, path string, mspID string) error {
	s.mspsMutex.Lock()
	defer s.mspsMutex.Unlock()

	provider, err := x509.NewProvider(path, mspID, s.signerService)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", path)
	}

	s.DeserializerManager().AddDeserializer(provider)
	s.AddMSP(id, BccspMSP, provider.EnrollmentID(), provider.Identity)

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

func (s *service) AddMSP(name string, mspType string, enrollmentID string, IdentityGetter fdriver.GetIdentityFunc) {
	if mspType == BccspMSP && s.binderService != nil {
		id, _, err := IdentityGetter(nil)
		if err != nil {
			panic(fmt.Sprintf("cannot get identity for [%s,%s,%s][%s]", name, mspType, enrollmentID, err))
		}
		if err := s.binderService.Bind(s.defaultViewIdentity, id); err != nil {
			panic(fmt.Sprintf("cannot bind identity for [%s,%s,%s][%s]", name, mspType, enrollmentID, err))
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
			panic(fmt.Sprintf("cannot get identity for [%s,%s,%s][%s]", name, mspType, enrollmentID, err))
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

func (s *service) DeserializerManager() driver.DeserializerManager {
	dm, err := s.sp.GetService(reflect.TypeOf((*driver.DeserializerManager)(nil)))
	if err != nil {
		panic(fmt.Sprintf("failed looking up deserializer manager [%s]", err))
	}
	return dm.(driver.DeserializerManager)
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
			return errors.WithMessagef(err, "failed to load idemix msp [%s]", config.ID)
		}
	}

	if s.defaultIdentity == nil {
		return errors.Errorf("no default identity set for network [%s]", s.config.Name())
	}

	return nil
}

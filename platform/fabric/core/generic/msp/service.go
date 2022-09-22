/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
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

type Config interface {
	Name() string
	DefaultMSP() string
	MSPs() ([]config.MSP, error)
	TranslatePath(path string) string
}

type SignerService interface {
	RegisterSigner(identity view.Identity, signer fdriver.Signer, verifier fdriver.Verifier) error
}

type BinderService interface {
	Bind(longTerm view.Identity, ephemeral view.Identity) error
}

type ConfigProvider interface {
	driver.ConfigService
}

type DeserializerManager interface {
	AddDeserializer(deserializer sig.Deserializer)
}

type Configuration struct {
	ID      string `yaml:"id"`
	MSPType string `yaml:"mspType"`
	MSPID   string `yaml:"mspID"`
	Path    string `yaml:"path"`
}

type MSP struct {
	Name         string `yaml:"name,omitempty"`
	Type         string `yaml:"type,omitempty"`
	EnrollmentID string
	GetIdentity  fdriver.GetIdentityFunc
}

type service struct {
	sp                     view2.ServiceProvider
	defaultIdentity        view.Identity
	defaultSigningIdentity x509.SigningIdentity
	signerService          SignerService
	binderService          BinderService
	defaultViewIdentity    view.Identity
	config                 Config

	mspsMutex           sync.RWMutex
	msps                []*MSP
	mspsByName          map[string]*MSP
	mspsByEnrollmentID  map[string]*MSP
	mspsByTypeAndName   map[string]*MSP
	bccspMspsByIdentity map[string]*MSP
	cacheSize           int
}

func NewLocalMSPManager(
	sp view2.ServiceProvider,
	config Config,
	signerService SignerService,
	binderService BinderService,
	defaultViewIdentity view.Identity,
	cacheSize int,
) *service {
	return &service{
		sp:                  sp,
		config:              config,
		signerService:       signerService,
		binderService:       binderService,
		defaultViewIdentity: defaultViewIdentity,
		mspsByTypeAndName:   map[string]*MSP{},
		bccspMspsByIdentity: map[string]*MSP{},
		mspsByEnrollmentID:  map[string]*MSP{},
		mspsByName:          map[string]*MSP{},
		cacheSize:           cacheSize,
	}
}

func (s *service) Load() error {
	if err := s.loadLocalMSPs(); err != nil {
		return err
	}
	return nil
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

	s.deserializerManager().AddDeserializer(provider)
	s.addMSP(id, IdemixMSP, provider.EnrollmentID(), NewIdentityCache(provider.Identity, s.cacheSize).Identity)
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

	s.deserializerManager().AddDeserializer(provider)
	s.addMSP(id, BccspMSP, provider.EnrollmentID(), provider.Identity)

	return nil
}

func (s *service) Refresh() error {
	s.mspsMutex.Lock()
	defer s.mspsMutex.Unlock()

	// clean cashes
	s.msps = nil
	s.mspsByTypeAndName = map[string]*MSP{}
	s.bccspMspsByIdentity = map[string]*MSP{}
	s.mspsByEnrollmentID = map[string]*MSP{}
	s.mspsByName = map[string]*MSP{}

	// reload
	return s.Load()
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

func (s *service) addMSP(Name string, Type string, EnrollmentID string, IdentityGetter fdriver.GetIdentityFunc) {
	if Type == BccspMSP && s.binderService != nil {
		id, _, err := IdentityGetter(nil)
		if err != nil {
			panic(fmt.Sprintf("cannot get identity for [%s,%s,%s][%s]", Name, Type, EnrollmentID, err))
		}
		if err := s.binderService.Bind(s.defaultViewIdentity, id); err != nil {
			panic(fmt.Sprintf("cannot bind identity for [%s,%s,%s][%s]", Name, Type, EnrollmentID, err))
		}
	}

	msp := &MSP{
		Name:         Name,
		Type:         Type,
		EnrollmentID: EnrollmentID,
		GetIdentity:  IdentityGetter,
	}
	if Type == BccspMSP {
		id, _, err := IdentityGetter(nil)
		if err != nil {
			panic(fmt.Sprintf("cannot get identity for [%s,%s,%s][%s]", Name, Type, EnrollmentID, err))
		}
		s.bccspMspsByIdentity[id.String()] = msp
		logger.Debugf("add bccsp msp for id %s, identity [%s]", Name+"@"+EnrollmentID, id.String())
	} else {
		logger.Debugf("add idemix msp for id %s", Name+"@"+EnrollmentID)
	}
	s.mspsByTypeAndName[Type+Name] = msp
	s.mspsByName[Name] = msp
	if len(EnrollmentID) != 0 {
		s.mspsByEnrollmentID[EnrollmentID] = msp
	}
	s.msps = append(s.msps, msp)
}

func (s *service) deserializerManager() DeserializerManager {
	dm, err := s.sp.GetService(reflect.TypeOf((*DeserializerManager)(nil)))
	if err != nil {
		panic(fmt.Sprintf("failed looking up deserializer manager [%s]", err))
	}
	return dm.(DeserializerManager)
}

func (s *service) loadLocalMSPs() error {
	configs, err := s.config.MSPs()
	if err != nil {
		return errors.WithMessagef(err, "failed loading local MSP configs")
	}
	defaultMSP := s.config.DefaultMSP()
	if len(defaultMSP) == 0 {
		if len(configs) == 0 {
			return errors.New("default MSP not configured and no MSPs set")
		}
		logger.Warnf("default MSP not configured, set it to [%s]", configs[0].ID)
		defaultMSP = configs[0].ID
	}

	logger.Debugf("Local Local [%d] MSPS using default [%s]", len(configs), defaultMSP)
	for _, config := range configs {
		switch config.MSPType {
		case IdemixMSP:
			if err := s.loadIdemixMSP(config); err != nil {
				return errors.WithMessagef(err, "failed to load idemix msp [%s]", config.ID)
			}
		case IdemixMSPFolder:
			if err := s.loadIdemixMSPFolder(config); err != nil {
				return errors.WithMessagef(err, "failed to load idemix msp folder [%s]", config.ID)
			}
		case BccspMSP:
			if err := s.loadBCCSPMSP(config.ID, config, defaultMSP); err != nil {
				return errors.WithMessagef(err, "failed loading bccsp msp [%s]", config.ID)
			}
		case BccspMSPFolder:
			if err := s.loadBCCSPMSPFolder(config, defaultMSP); err != nil {
				return errors.WithMessagef(err, "failed loading bccsp msp folder [%s]", config.Path)
			}
		default:
			logger.Warnf("msp type [%s] not recognized, skipping", config.MSPType)
			continue
		}
	}

	if s.defaultIdentity == nil {
		return errors.Errorf("no default identity set for network [%s]", s.config.Name())
	}

	return nil
}

func (s *service) loadBCCSPMSP(id string, c config.MSP, defaultMSP string) error {
	// Try without "msp"
	var bccspOpts *config.BCCSP
	if c.Opts != nil {
		bccspOpts = c.Opts.BCCSP
	}
	provider, err := x509.NewProviderWithBCCSPConfig(
		s.config.TranslatePath(c.Path),
		c.MSPID,
		s.signerService,
		bccspOpts,
	)
	if err != nil {
		logger.Warnf("failed reading bccsp msp configuration from [%s]: [%s]", filepath.Join(s.config.TranslatePath(c.Path), id), err)
		// Try with "msp"
		provider, err = x509.NewProviderWithBCCSPConfig(
			filepath.Join(s.config.TranslatePath(c.Path), "msp"),
			c.MSPID,
			s.signerService,
			bccspOpts,
		)
		if err != nil {
			logger.Warnf("failed reading bccsp msp configuration from [%s and %s]: [%s]",
				filepath.Join(s.config.TranslatePath(c.Path),
					filepath.Join(s.config.TranslatePath(c.Path), "msp")), err,
			)
			return errors.WithMessagef(err, "failed to load BCCSP MSP configuration [%s]", id)
		}
	}

	s.deserializerManager().AddDeserializer(provider)
	s.addMSP(c.ID, c.MSPType, provider.EnrollmentID(), provider.Identity)
	if id == defaultMSP {
		if s.defaultIdentity == nil {
			logger.Infof("setting default identity to [%s]", c.ID)
		}

		// set default
		s.defaultIdentity, _, err = provider.Identity(nil)
		if err != nil {
			return errors.WithMessagef(err, "failed to get default identity for [%s]", c.MSPID)
		}
		s.defaultSigningIdentity, err = provider.SerializedIdentity()
		if err != nil {
			return errors.WithMessagef(err, "failed to get default signing identity for [%s]", c.MSPID)
		}
	}
	return nil
}

func (s *service) loadIdemixMSP(config config.MSP) error {
	conf, err := msp.GetLocalMspConfigWithType(s.config.TranslatePath(config.Path), nil, config.MSPID, config.MSPType)
	if err != nil {
		return errors.Wrapf(err, "failed reading idemix msp configuration from [%s]", s.config.TranslatePath(config.Path))
	}
	provider, err := idemix.NewAnyProvider(conf, s.sp)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", s.config.TranslatePath(config.Path))
	}
	s.deserializerManager().AddDeserializer(provider)
	cacheSize := s.cacheSize
	if config.CacheSize > 0 {
		cacheSize = config.CacheSize
	}
	s.addMSP(config.ID, config.MSPType, provider.EnrollmentID(), NewIdentityCache(provider.Identity, cacheSize).Identity)
	logger.Debugf("added %s msp for id %s with cache of size %d", config.MSPType, config.ID+"@"+provider.EnrollmentID(), cacheSize)

	return nil
}

func (s *service) loadBCCSPMSPFolder(mspConfig config.MSP, defaultMSP string) error {
	entries, err := ioutil.ReadDir(s.config.TranslatePath(mspConfig.Path))
	if err != nil {
		logger.Warnf("failed reading from [%s]: [%s]", s.config.TranslatePath(mspConfig.Path), err)
		return errors.Wrapf(err, "failed reading from [%s]", s.config.TranslatePath(mspConfig.Path))
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()

		if err := s.loadBCCSPMSP(id, config.MSP{
			ID:      id,
			MSPType: BccspMSP,
			MSPID:   id,
			Path:    filepath.Join(s.config.TranslatePath(mspConfig.Path), id),
			Opts:    mspConfig.Opts,
		}, defaultMSP); err != nil {
			return errors.WithMessagef(err, "failed to load BCCSP MSP configuration [%s]", id)
		}
	}
	return nil
}

func (s *service) loadIdemixMSPFolder(mspConfig config.MSP) error {
	entries, err := ioutil.ReadDir(s.config.TranslatePath(mspConfig.Path))
	if err != nil {
		logger.Warnf("failed reading from [%s]: [%s]", s.config.TranslatePath(mspConfig.Path), err)
		return errors.Wrapf(err, "failed reading from [%s]", s.config.TranslatePath(mspConfig.Path))
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()

		if err := s.loadIdemixMSP(config.MSP{
			ID:      id,
			MSPType: IdemixMSP,
			MSPID:   id,
			Path:    filepath.Join(s.config.TranslatePath(mspConfig.Path), id),
		}); err != nil {
			return errors.WithMessagef(err, "failed to load Idemix MSP configuration [%s]", id)
		}
	}
	return nil
}

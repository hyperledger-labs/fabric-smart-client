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
	idemix2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	x5092 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	api2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"

	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	sig2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	api3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	IdemixMSP       = "idemix"
	IdemixMSPFolder = "idemix-folder"
	BccspMSP        = "bccsp"
	BccspMSPFolder  = "bccsp-folder"
)

var logger = flogging.MustGetLogger("fabric-sdk.msp")

type Config interface {
	MSPConfigPath() string
	MSPs() ([]config.MSP, error)
	LocalMSPID() string
	LocalMSPType() string
	TranslatePath(path string) string
}

type SignerService interface {
	RegisterSigner(identity view.Identity, signer api2.Signer, verifier api2.Verifier) error
}

type BinderService interface {
	Bind(longTerm view.Identity, ephemeral view.Identity) error
}

type ConfigProvider interface {
	api3.ConfigService
}

type DeserializerManager interface {
	AddDeserializer(deserializer sig2.Deserializer)
}

type Configuration struct {
	ID      string `yaml:"id"`
	MSPType string `yaml:"mspType"`
	MSPID   string `yaml:"mspID"`
	Path    string `yaml:"path"`
}

type Resolver struct {
	Name         string `yaml:"name,omitempty"`
	Type         string `yaml:"type,omitempty"`
	EnrollmentID string
	GetIdentity  api2.GetIdentityFunc
}

type service struct {
	sp                     view2.ServiceProvider
	defaultIdentity        view.Identity
	defaultSigningIdentity x5092.SigningIdentity
	signerService          SignerService
	binderService          BinderService
	defaultViewIdentity    view.Identity
	config                 Config

	resolversMutex           sync.RWMutex
	resolvers                []*Resolver
	resolversByName          map[string]*Resolver
	resolversByEnrollmentID  map[string]*Resolver
	resolversByTypeAndName   map[string]*Resolver
	bccspResolversByIdentity map[string]*Resolver
	cacheSize                int
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
		sp:                       sp,
		config:                   config,
		signerService:            signerService,
		binderService:            binderService,
		defaultViewIdentity:      defaultViewIdentity,
		resolversByTypeAndName:   map[string]*Resolver{},
		bccspResolversByIdentity: map[string]*Resolver{},
		resolversByEnrollmentID:  map[string]*Resolver{},
		resolversByName:          map[string]*Resolver{},
		cacheSize:                cacheSize,
	}
}

func (s *service) Load() error {
	// Default Resolver
	if err := s.loadDefaultResolver(); err != nil {
		return err
	}

	// Register extra Resolvers
	if err := s.loadExtraResolvers(); err != nil {
		return err
	}

	return nil
}

func (s *service) GetDefaultIdentity() (view.Identity, x5092.SigningIdentity) {
	return s.defaultIdentity, s.defaultSigningIdentity
}

func (s *service) DefaultIdentity() view.Identity {
	return s.defaultIdentity
}

func (s *service) AnonymousIdentity() view.Identity {
	id := s.Identity("idemix")

	resolver := view2.GetEndpointService(s.sp)
	if err := resolver.Bind(s.defaultViewIdentity, id); err != nil {
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

func (s *service) DefaultSigningIdentity() api2.SigningIdentity {
	return s.defaultSigningIdentity
}

func (s *service) GetIdentityInfoByLabel(mspType string, label string) *api2.IdentityInfo {
	s.resolversMutex.RLock()
	defer s.resolversMutex.RUnlock()

	logger.Debugf("get identity info by label [%s:%s]", mspType, label)
	r, ok := s.resolversByTypeAndName[mspType+label]
	if !ok {
		logger.Debugf("identity info not found for label [%s:%s][%v]", mspType, label, s.resolversByTypeAndName)
		return nil
	}
	return &api2.IdentityInfo{
		ID:           r.Name,
		EnrollmentID: r.EnrollmentID,
		GetIdentity:  r.GetIdentity,
	}
}

func (s *service) GetIdentityInfoByIdentity(mspType string, id view.Identity) *api2.IdentityInfo {
	s.resolversMutex.RLock()
	defer s.resolversMutex.RUnlock()

	if mspType == BccspMSP {
		r, ok := s.bccspResolversByIdentity[id.String()]
		if !ok {
			return nil
		}
		return &api2.IdentityInfo{
			ID:           r.Name,
			EnrollmentID: r.EnrollmentID,
			GetIdentity:  r.GetIdentity,
		}
	}

	// scan all resolvers in the worst case
	for _, r := range s.resolvers {
		if r.Type == mspType {
			lid, _, err := r.GetIdentity(nil)
			if err != nil {
				continue
			}
			if id.Equal(lid) {
				return &api2.IdentityInfo{
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
	s.resolversMutex.RLock()
	defer s.resolversMutex.RUnlock()

	// Check indices first
	r, ok := s.resolversByName[id]
	if ok {
		identity, _, err := r.GetIdentity(nil)
		return identity, err
	}

	r, ok = s.resolversByEnrollmentID[id]
	if ok {
		identity, _, err := r.GetIdentity(nil)
		return identity, err
	}

	// Scan
	for _, r := range s.resolvers {
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
	s.resolversMutex.Lock()
	defer s.resolversMutex.Unlock()

	conf, err := msp.GetLocalMspConfigWithType(path, nil, mspID, IdemixMSP)
	if err != nil {
		return errors.Wrapf(err, "failed reading idemix msp configuration from [%s]", path)
	}
	provider, err := idemix2.NewAnyProvider(conf, s.sp)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", path)
	}

	s.deserializerManager().AddDeserializer(provider)
	s.addResolver(id, IdemixMSP, provider.EnrollmentID(), NewIdentityCache(provider.Identity, s.cacheSize).Identity)
	logger.Debugf("added IdemixMSP resolver for id %s with cache of size %d", id+"@"+provider.EnrollmentID(), s.cacheSize)
	return nil
}

func (s *service) RegisterX509MSP(id string, path string, mspID string) error {
	s.resolversMutex.Lock()
	defer s.resolversMutex.Unlock()

	provider, err := x5092.NewProvider(path, mspID, s.signerService)
	if err != nil {
		return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", path)
	}

	s.deserializerManager().AddDeserializer(provider)
	s.addResolver(id, BccspMSP, provider.EnrollmentID(), provider.Identity)

	return nil
}

func (s *service) Refresh() error {
	s.resolversMutex.Lock()
	defer s.resolversMutex.Unlock()

	// clean cashes
	s.resolvers = nil
	s.resolversByTypeAndName = map[string]*Resolver{}
	s.bccspResolversByIdentity = map[string]*Resolver{}
	s.resolversByEnrollmentID = map[string]*Resolver{}
	s.resolversByName = map[string]*Resolver{}

	// reload
	return s.Load()
}

func (s *service) Resolvers() []string {
	s.resolversMutex.RLock()
	defer s.resolversMutex.RUnlock()

	var res []string
	for _, r := range s.resolvers {
		res = append(res, r.Name)
	}
	return res
}

func (s *service) addResolver(Name string, Type string, EnrollmentID string, IdentityGetter api2.GetIdentityFunc) {
	if Type == BccspMSP && s.binderService != nil {
		id, _, err := IdentityGetter(nil)
		if err != nil {
			panic(fmt.Sprintf("cannot get identity for [%s,%s,%s][%s]", Name, Type, EnrollmentID, err))
		}
		if err := s.binderService.Bind(s.defaultViewIdentity, id); err != nil {
			panic(fmt.Sprintf("cannot bind identity for [%s,%s,%s][%s]", Name, Type, EnrollmentID, err))
		}
	}

	resolver := &Resolver{
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
		s.bccspResolversByIdentity[id.String()] = resolver
	}
	s.resolversByTypeAndName[Type+Name] = resolver
	s.resolversByName[Name] = resolver
	if len(EnrollmentID) != 0 {
		s.resolversByEnrollmentID[EnrollmentID] = resolver
	}
	s.resolvers = append(s.resolvers, resolver)
}

func (s *service) deserializerManager() DeserializerManager {
	dm, err := s.sp.GetService(reflect.TypeOf((*DeserializerManager)(nil)))
	if err != nil {
		panic(fmt.Sprintf("failed looking up deserializer manager [%s]", err))
	}
	return dm.(DeserializerManager)
}

func (s *service) loadDefaultResolver() error {
	var mspConfigDir = s.config.MSPConfigPath()
	if len(mspConfigDir) == 0 {
		logger.Warnf("default msp config not set, this might have an impact on the rest of the system")
		return nil
	}

	var mspID = s.config.LocalMSPID()
	var mspType = s.config.LocalMSPType()
	switch {
	case mspType == "":
		mspType = msp.ProviderTypeToString(msp.FABRIC)
	case mspType != msp.ProviderTypeToString(msp.FABRIC):
		return errors.Errorf("default identity must by of type [%s]", msp.ProviderTypeToString(msp.FABRIC))
	}
	provider, err := x5092.NewProvider(mspConfigDir, mspID, s.signerService)
	if err != nil {
		return err
	}
	s.defaultSigningIdentity, err = provider.SerializedIdentity()
	if err != nil {
		return err
	}
	s.defaultIdentity, _, err = provider.Identity(nil)
	if err != nil {
		return err
	}

	s.addResolver("default", mspType, provider.EnrollmentID(), provider.Identity)
	s.deserializerManager().AddDeserializer(provider)

	return nil
}

func (s *service) loadExtraResolvers() error {
	configs, err := s.config.MSPs()
	if err != nil {
		return err
	}
	logger.Debugf("Found extra identities %d", len(configs))

	type Provider interface {
		EnrollmentID() string
		Identity(opts *api2.IdentityOptions) (view.Identity, []byte, error)
		DeserializeVerifier(raw []byte) (driver.Verifier, error)
		DeserializeSigner(raw []byte) (driver.Signer, error)
		Info(raw []byte, auditInfo []byte) (string, error)
	}
	var provider Provider
	dm := s.deserializerManager()

	for _, config := range configs {
		cacheSize := s.cacheSize
		if config.CacheSize > 0 {
			cacheSize = config.CacheSize
		}

		switch config.MSPType {
		case IdemixMSP:
			conf, err := msp.GetLocalMspConfigWithType(s.config.TranslatePath(config.Path), nil, config.MSPID, config.MSPType)
			if err != nil {
				return errors.Wrapf(err, "failed reading idemix msp configuration from [%s]", s.config.TranslatePath(config.Path))
			}
			provider, err = idemix2.NewAnyProvider(conf, s.sp)
			if err != nil {
				return errors.Wrapf(err, "failed instantiating idemix msp provider from [%s]", s.config.TranslatePath(config.Path))
			}
			dm.AddDeserializer(provider)
			s.addResolver(config.ID, config.MSPType, provider.EnrollmentID(), NewIdentityCache(provider.Identity, cacheSize).Identity)
			logger.Debugf("added %s resolver for id %s with cache of size %d", config.MSPType, config.ID+"@"+provider.EnrollmentID(), cacheSize)
		case BccspMSP:
			provider, err = x5092.NewProvider(s.config.TranslatePath(config.Path), config.MSPID, s.signerService)
			if err != nil {
				return errors.Wrapf(err, "failed instantiating x509 msp provider from [%s]", s.config.TranslatePath(config.Path))
			}
			dm.AddDeserializer(provider)
			s.addResolver(config.ID, config.MSPType, provider.EnrollmentID(), provider.Identity)
		case IdemixMSPFolder:
			entries, err := ioutil.ReadDir(s.config.TranslatePath(config.Path))
			if err != nil {
				logger.Warnf("failed reading from [%s]: [%s]", s.config.TranslatePath(config.Path), err)
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				id := entry.Name()
				conf, err := msp.GetLocalMspConfigWithType(
					filepath.Join(s.config.TranslatePath(config.Path), id),
					nil,
					config.MSPID,
					IdemixMSP,
				)
				if err != nil {
					logger.Warnf("failed reading idemix msp configuration from [%s]: [%s]", filepath.Join(s.config.TranslatePath(config.Path), id), err)
					continue
				}
				provider, err = idemix2.NewAnyProvider(conf, s.sp)
				if err != nil {
					logger.Warnf("failed instantiating idemix msp configuration from [%s]: [%s]", filepath.Join(s.config.TranslatePath(config.Path), id), err)
					continue
				}
				dm.AddDeserializer(provider)
				logger.Debugf("Adding resolver [%s:%s]", id, provider.EnrollmentID())
				s.addResolver(id, IdemixMSP, provider.EnrollmentID(), NewIdentityCache(provider.Identity, cacheSize).Identity)
				logger.Debugf("added %s resolver for id %s with cache of size %d", IdemixMSP, id+"@"+provider.EnrollmentID(), cacheSize)
			}
		case BccspMSPFolder:
			entries, err := ioutil.ReadDir(s.config.TranslatePath(config.Path))
			if err != nil {
				logger.Warnf("failed reading from [%s]: [%s]", s.config.TranslatePath(config.Path), err)
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				id := entry.Name()

				// Try without "msp"
				provider, err = x5092.NewProvider(
					filepath.Join(s.config.TranslatePath(config.Path), id),
					config.MSPID,
					s.signerService,
				)
				if err != nil {
					logger.Debugf("failed reading bccsp msp configuration from [%s]: [%s]", filepath.Join(s.config.TranslatePath(config.Path), id), err)
					// Try with "msp"
					provider, err = x5092.NewProvider(
						filepath.Join(s.config.TranslatePath(config.Path), id, "msp"),
						config.MSPID,
						s.signerService,
					)
					if err != nil {
						logger.Warnf("failed reading bccsp msp configuration from [%s and %s]: [%s]",
							filepath.Join(s.config.TranslatePath(config.Path),
								filepath.Join(s.config.TranslatePath(config.Path), id, "msp")), err,
						)
						continue
					}
				}

				dm.AddDeserializer(provider)
				logger.Debugf("Adding resolver [%s:%s]", id, provider.EnrollmentID())
				s.addResolver(id, BccspMSP, provider.EnrollmentID(), provider.Identity)
			}
		default:
			logger.Warnf("msp type [%s] not recognized, skipping", config.MSPType)
			continue
		}
	}
	return nil
}

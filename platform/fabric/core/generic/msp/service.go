/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/deferred"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	IdemixMSP       = "idemix"
	IdemixMSPFolder = "idemix-folder"
	BccspMSP        = "bccsp"
	BccspMSPFolder  = "bccsp-folder"
)

var logger = logging.MustGetLogger()

type KVS interface {
	Exists(ctx context.Context, id string) bool
	Put(ctx context.Context, id string, state any) error
	Get(ctx context.Context, id string, state any) error
}

// defaultIdentity pairs the network's default identity with the signing identity
// that goes with it. SetDefaultIdentity installs the two together and they are
// meaningless apart, so they are held together rather than as two fields that
// could drift.
type defaultIdentity struct {
	id      view.Identity
	signing driver.SigningIdentity
}

type service struct {
	// defaults holds the default identity once an identity loader has supplied
	// one and it was accepted. It is not available when the service is built: it
	// arrives while loadLocalMSPs walks the configured MSPs, so until then the
	// holder is empty and reports which kind of empty it is — never offered one,
	// or offered one and refused it. loadLocalMSPs turns either into a startup
	// failure rather than letting a nil identity reach a caller that cannot tell
	// it apart from a network with no default.
	defaults *deferred.Holder[defaultIdentity]

	// defaultMSP holds the identifier of the MSP whose identity becomes the
	// network default. It is resolved by loadLocalMSPs, from the configured
	// value or, when nothing is configured, from the first MSP in the list, so
	// it is not the same as the configured value that Config.DefaultMSP
	// reports.
	//
	// Held rather than guarded by mspsMutex because SetDefaultIdentity reads it
	// and is reachable through the exported driver.Manager interface: identity
	// loaders are a dependency-injection extension point, so a loader outside
	// this repository can hold a Manager and call it from a goroutine that owns
	// no lock of ours. A holder is safe for any caller, whereas taking
	// mspsMutex there would deadlock the in-tree loaders, which are already
	// inside that critical section.
	//
	// Get also distinguishes "not resolved yet" from a real identifier, which
	// an empty string cannot: MSP identifiers are nowhere validated as
	// non-empty, so before loadLocalMSPs has run an empty id would otherwise
	// compare equal and install itself as the default.
	defaultMSP *deferred.Holder[string]

	signerService       driver.SignerService
	binderService       driver.BinderService
	deserializerManager driver.DeserializerManager
	defaultViewIdentity view.Identity
	KVS                 KVS
	config              driver.Config

	// mspsMutex guards the fields below. They are written by loadLocalMSPs and
	// by AddMSP, which an identity loader calls from inside that same critical
	// section on the same goroutine, so AddMSP takes no lock of its own and
	// must not start taking one — sync.RWMutex is not reentrant and it would
	// deadlock.
	//
	// That is a known hole rather than a safe convention. AddMSP is on the
	// exported driver.Manager interface and identity loaders are a
	// dependency-injection extension point, so a loader outside this repository
	// can retain a Manager and call AddMSP from a goroutine holding no lock of
	// ours, racing the readers below — a concurrent map write, which is fatal
	// rather than merely wrong. Fixing it means giving AddMSP a lock that
	// loadLocalMSPs does not already hold, which is a change of its own.
	mspsMutex           sync.RWMutex
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
		defaults:            deferred.NewHolder[defaultIdentity]("default identity"),
		defaultMSP:          deferred.NewHolder[string]("default MSP"),
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

// SetDefaultIdentity installs id's identity as the network default, if id is the
// MSP resolved as the default one. Identities from any other MSP are ignored,
// and an empty identity is refused.
//
// Identity loaders call this from inside loadLocalMSPs, but the method is also
// reachable through driver.Manager, so it holds no assumption about which locks
// its caller owns; see the defaultMSP field.
func (s *service) SetDefaultIdentity(id string, identity view.Identity, signing driver.SigningIdentity) {
	defaultMSP, err := s.defaultMSP.Get()
	if err != nil {
		// Called before loadLocalMSPs resolved which MSP is the default, so
		// there is nothing to compare id against yet.
		logger.Warnf("ignoring default identity from MSP [%s]: %v", id, err)
		return
	}

	if id != defaultMSP {
		return
	}

	// Refusing the update leaves the holder empty and records why, so
	// loadLocalMSPs fails with that reason. Installing an empty identity instead
	// would satisfy its check and hand nil to every caller of DefaultIdentity,
	// turning a startup failure into a signing failure much later on.
	if err := s.defaults.Update(func(defaultIdentity, bool) (defaultIdentity, error) {
		// Both halves are checked, because both are handed out: an identity
		// without its signer would satisfy loadLocalMSPs and then fail at the
		// first signature instead, in whatever component called
		// DefaultSigningIdentity.
		if identity.IsNone() {
			return defaultIdentity{}, errors.Errorf("MSP [%s] supplied an empty identity", id)
		}
		if signing == nil {
			return defaultIdentity{}, errors.Errorf("MSP [%s] supplied no signing identity", id)
		}
		return defaultIdentity{id: identity, signing: signing}, nil
	}); err != nil {
		logger.Warnf("refused default identity: %v", err)
		return
	}

	logger.Debugf("set default identity to [%s]", id)
}

// DefaultIdentity returns the network's default identity, or nil if no identity
// loader has supplied one yet.
func (s *service) DefaultIdentity() view.Identity {
	d, _ := s.defaults.TryGet()
	return d.id
}

func (s *service) AnonymousIdentity() (view.Identity, error) {
	id, err := s.Identity("idemix")
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get default anonymous identity labelled `idemix`")
	}
	if err := s.binderService.Bind(context.Background(), s.defaultViewIdentity, id); err != nil {
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

func (s *service) IsMe(ctx context.Context, id view.Identity) bool {
	return s.signerService.IsMe(ctx, id)
}

// DefaultSigningIdentity returns the signing identity that goes with the
// network's default identity, or nil if no identity loader has supplied one yet.
func (s *service) DefaultSigningIdentity() fdriver.SigningIdentity {
	d, _ := s.defaults.TryGet()
	return d.signing
}

func (s *service) GetIdentityInfoByLabel(mspType, label string) *fdriver.IdentityInfo {
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
	if err != nil || identity == nil {
		return nil, errors.Errorf("identity [%s] not found", id)
	}
	return identity, nil
}

func (s *service) RegisterIdemixMSP(id, path, mspID string) error {
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
	if err := s.AddMSP(id, IdemixMSP, provider.EnrollmentID(), idemix.NewIdentityCache(provider.Identity, s.cacheSize, nil, nil).Identity); err != nil {
		return errors.Wrapf(err, "failed adding idemix msp [%s] to [%s]", id, path)
	}
	logger.Debugf("added IdemixMSP msp for id %s with cache of size %d", id+"@"+provider.EnrollmentID(), s.cacheSize)
	return nil
}

func (s *service) RegisterX509MSP(id, path, mspID string) error {
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

// Refresh discards everything derived from the configured MSPs and builds it
// again, so that identities added or removed since the last load are picked up.
//
// It fails, leaving the service without a default identity, if the reload cannot
// produce one. Keeping the previous identity across a reload that dropped its
// MSP would leave the service signing as an MSP it no longer knows about, and
// would also hide the reason the reload produced nothing, since the holder only
// records a refusal while nothing is held.
func (s *service) Refresh() error {
	s.mspsMutex.Lock()
	defer s.mspsMutex.Unlock()

	// clean caches
	s.msps = nil
	s.mspsByTypeAndName = map[string]*driver.MSP{}
	s.bccspMspsByIdentity = map[string]*driver.MSP{}
	s.mspsByEnrollmentID = map[string]*driver.MSP{}
	s.mspsByName = map[string]*driver.MSP{}
	s.defaults.Reset()

	return s.loadLocalMSPs()
}

func (s *service) AddMSP(name, mspType, enrollmentID string, IdentityGetter fdriver.GetIdentityFunc) error {
	if mspType == BccspMSP && s.binderService != nil {
		id, _, err := IdentityGetter(nil)
		if err != nil {
			return errors.Wrapf(err, "cannot get identity for [%s,%s,%s][%s]", name, mspType, enrollmentID, err)
		}
		if err := s.binderService.Bind(context.Background(), s.defaultViewIdentity, id); err != nil {
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
	defaultMSP := s.config.DefaultMSP()
	if len(defaultMSP) == 0 {
		if len(configs) == 0 {
			return errors.New("default MSP not configured and no MSPs set")
		}
		logger.Warnf("default MSP not configured, set it to [%s]", configs[0].ID)
		defaultMSP = configs[0].ID
	}

	// Resolved before the loaders run, because each of them calls
	// SetDefaultIdentity, which needs it to decide whether its MSP is the one.
	// The error is discarded because this update cannot fail: it accepts
	// whatever was resolved above.
	_ = s.defaultMSP.Update(func(string, bool) (string, error) {
		return defaultMSP, nil
	})

	logger.Debugf("Local Local [%d] MSPS using default [%s]", len(configs), defaultMSP)
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

	return s.defaultIdentityError()
}

// defaultIdentityError reports why the loaders left no default identity behind,
// or nil if they installed one.
//
// Both outcomes are a permanent misconfiguration, so the holder's error is
// folded in as text rather than wrapped: an unoffered default carries
// driver.ErrNotInitialized, which callers read as "still starting up, retry",
// and no retry produces a default the loaders did not supply on this pass.
//
// Refresh does not clear the holder, so on that path a default accepted by an
// earlier load still satisfies this check.
func (s *service) defaultIdentityError() error {
	_, err := s.defaults.Get()
	switch {
	case err == nil:
		return nil
	case errors.Is(err, fdriver.ErrConfigRejected):
		// The default MSP did offer an identity and it was refused. Say why.
		return errors.Errorf("no usable default identity for network [%s]: %s", s.config.NetworkName(), err)
	default:
		// No loader offered one: the configured default MSP has no identity, or
		// no loader recognised its type.
		return errors.Errorf("no default identity set for network [%s]", s.config.NetworkName())
	}
}

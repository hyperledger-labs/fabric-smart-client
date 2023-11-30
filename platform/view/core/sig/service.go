/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sig

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("view-sdk.sig")

type KVS interface {
	Exists(id string) bool
	Put(id string, state interface{}) error
	Get(id string, state interface{}) error
}

type VerifierEntry struct {
	Verifier   driver.Verifier
	DebugStack []byte
}

type SignerEntry struct {
	Signer     driver.Signer
	DebugStack []byte
}

type Service struct {
	sp           driver.ServiceProvider
	signers      map[string]SignerEntry
	verifiers    map[string]VerifierEntry
	deserializer Deserializer
	viewsSync    sync.RWMutex
	kvs          KVS
}

func NewSignService(sp driver.ServiceProvider, deserializer Deserializer, kvs KVS) *Service {
	return &Service{
		sp:           sp,
		signers:      map[string]SignerEntry{},
		verifiers:    map[string]VerifierEntry{},
		deserializer: deserializer,
		kvs:          kvs,
	}
}

func (o *Service) RegisterSigner(identity view.Identity, signer driver.Signer, verifier driver.Verifier) error {
	if signer == nil {
		return errors.New("invalid signer, expected a valid instance")
	}

	idHash := identity.UniqueID()
	o.viewsSync.Lock()
	s, ok := o.signers[idHash]
	o.viewsSync.Unlock()
	if ok {
		logger.Warnf("another signer bound to [%s]:[%s][%s] from [%s]", identity, GetIdentifier(s), GetIdentifier(signer), string(s.DebugStack))
		return nil
	}
	o.viewsSync.Lock()

	entry := SignerEntry{Signer: signer}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		entry.DebugStack = debug.Stack()
	}

	o.signers[idHash] = entry
	if o.kvs != nil {
		k, err := kvs.CreateCompositeKey("sigService", []string{"signer", idHash})
		if err != nil {
			return errors.Wrap(err, "failed to create composite key to store entry in kvs")
		}
		err = o.kvs.Put(k, signer)
		if err != nil {
			return errors.Wrap(err, "failed to store entry in kvs for the passed signer")
		}
	}
	o.viewsSync.Unlock()

	if verifier != nil {
		return o.RegisterVerifier(identity, verifier)
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("signer for [%s][%s] registered, no verifier passed", idHash, GetIdentifier(signer))
	}
	return nil
}

func (o *Service) RegisterVerifier(identity view.Identity, verifier driver.Verifier) error {
	if verifier == nil {
		return errors.New("invalid verifier, expected a valid instance")
	}

	idHash := identity.UniqueID()
	o.viewsSync.Lock()
	v, ok := o.verifiers[idHash]
	o.viewsSync.Unlock()
	if ok {
		logger.Warnf("another verifier bound to [%s]:[%s][%s] from [%s]", idHash, GetIdentifier(v), GetIdentifier(verifier), string(v.DebugStack))
		return nil
	}
	entry := VerifierEntry{Verifier: verifier}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		entry.DebugStack = debug.Stack()
	}
	o.viewsSync.Lock()
	o.verifiers[idHash] = entry
	o.viewsSync.Unlock()
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("register verifier to [%s]:[%s]\n[%s]", idHash, GetIdentifier(verifier))
	}

	return nil
}

func (o *Service) RegisterAuditInfo(identity view.Identity, info []byte) error {
	k := kvs.CreateCompositeKeyOrPanic(
		"fsc.platform.view.sig",
		[]string{
			identity.String(),
		},
	)
	kvss := kvs.GetService(o.sp)
	if err := kvss.Put(k, info); err != nil {
		return err
	}
	return nil
}

func (o *Service) GetAuditInfo(identity view.Identity) ([]byte, error) {
	k := kvs.CreateCompositeKeyOrPanic(
		"fsc.platform.view.sig",
		[]string{
			identity.String(),
		},
	)
	kvss := kvs.GetService(o.sp)
	if !kvss.Exists(k) {
		return nil, nil
	}
	var res []byte
	if err := kvss.Get(k, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (o *Service) IsMe(identity view.Identity) bool {
	idHash := identity.UniqueID()
	// check local cache
	o.viewsSync.Lock()
	_, ok := o.signers[idHash]
	o.viewsSync.Unlock()
	if ok {
		return true
	}
	// check kvs
	if o.kvs != nil {
		k, err := kvs.CreateCompositeKey("sigService", []string{"signer", idHash})
		if err != nil {
			return false
		}
		if o.kvs.Exists(k) {
			return true
		}
	}
	// last chance, deserialize
	//signer, err := o.GetSigner(identity)
	//if err != nil {
	//	return false
	//}
	//return signer != nil
	return false
}

func (o *Service) Info(id view.Identity) string {
	auditInfo, err := o.GetAuditInfo(id)
	if err != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("failed getting audit info for [%s]", id)
		}
		return fmt.Sprintf("unable to identify identity : [%s][%s]", id.UniqueID(), string(id))
	}
	info, err := o.deserializer.Info(id, auditInfo)
	if err != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("failed getting info for [%s]", id)
		}
		return fmt.Sprintf("unable to identify identity : [%s][%s]", id.UniqueID(), string(id))
	}
	return info
}

func (o *Service) GetSigner(identity view.Identity) (driver.Signer, error) {
	idHash := identity.UniqueID()
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("get signer for [%s]", idHash)
	}
	o.viewsSync.Lock()
	entry, ok := o.signers[idHash]
	o.viewsSync.Unlock()
	if !ok {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("signer for [%s] not found, try to deserialize", idHash)
		}
		// ask the deserializer
		if o.deserializer == nil {
			return nil, errors.Errorf("cannot find signer for [%s], no deserializer set", identity)
		}
		var err error
		signer, err := o.deserializer.DeserializeSigner(identity)
		if err != nil {
			return nil, errors.Wrapf(err, "failed deserializing identity for signer [%s]", identity)
		}
		entry = SignerEntry{Signer: signer}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			entry.DebugStack = debug.Stack()
		}

		o.viewsSync.Lock()
		o.signers[idHash] = entry
		o.viewsSync.Unlock()
	} else {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("signer for [%s] found", idHash)
		}
	}
	return entry.Signer, nil
}

func (o *Service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	idHash := identity.UniqueID()
	o.viewsSync.Lock()
	entry, ok := o.verifiers[idHash]
	o.viewsSync.Unlock()
	if !ok {
		// ask the deserializer
		if o.deserializer == nil {
			return nil, errors.Errorf("cannot find verifier for [%s], no deserializer set", identity)
		}
		var err error
		verifier, err := o.deserializer.DeserializeVerifier(identity)
		if err != nil {
			return nil, errors.Wrapf(err, "failed deserializing identity for verifier %v", identity)
		}

		entry = VerifierEntry{Verifier: verifier}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			entry.DebugStack = debug.Stack()
			logger.Debugf("add deserialized verifier for [%s]:[%s]", idHash, GetIdentifier(verifier))
		}
		o.viewsSync.Lock()
		o.verifiers[idHash] = entry
		o.viewsSync.Unlock()
	}
	return entry.Verifier, nil
}

func (o *Service) GetSigningIdentity(identity view.Identity) (driver.SigningIdentity, error) {
	signer, err := o.GetSigner(identity)
	if err != nil {
		return nil, err
	}

	if _, ok := signer.(driver.SigningIdentity); ok {
		return signer.(driver.SigningIdentity), nil
	}

	return &si{
		id:     identity,
		signer: signer,
	}, nil
}

type si struct {
	id     view.Identity
	signer driver.Signer
}

func (s *si) Verify(message []byte, signature []byte) error {
	panic("implement me")
}

func (s *si) GetPublicVersion() driver.Identity {
	return s
}

func (s *si) Sign(bytes []byte) ([]byte, error) {
	return s.signer.Sign(bytes)
}

func (s *si) Serialize() ([]byte, error) {
	return s.id, nil
}

func GetIdentifier(f any) string {
	if f == nil {
		return "<nil>"
	}
	t := reflect.TypeOf(f)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() + "/" + t.Name()
}

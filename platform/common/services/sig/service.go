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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = logging.MustGetLogger("common-sdk.sig")

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
	deserializer Deserializer
	kvs          KVS

	mutex     sync.RWMutex
	signers   map[string]SignerEntry
	verifiers map[string]VerifierEntry
}

func NewService(deserializer Deserializer, kvs KVS) *Service {
	return &Service{
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

	// check existence with a read lock
	idHash := identity.UniqueID()
	o.mutex.RLock()
	s, ok := o.signers[idHash]
	o.mutex.RUnlock()
	if ok {
		logger.Debugf("another signer bound to [%s]:[%s][%s] from [%s]", identity, GetIdentifier(s), GetIdentifier(signer), string(s.DebugStack))
		return nil
	}

	// write lock
	o.mutex.Lock()

	// check again
	s, ok = o.signers[idHash]
	if ok {
		o.mutex.Unlock()
		logger.Debugf("another signer bound to [%s]:[%s][%s] from [%s]", identity, GetIdentifier(s), GetIdentifier(signer), string(s.DebugStack))
		return nil
	}

	entry := SignerEntry{Signer: signer}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		entry.DebugStack = debug.Stack()
	}
	o.signers[idHash] = entry
	o.mutex.Unlock()

	if o.kvs != nil {
		if err := o.registerSigner(idHash, &entry); err != nil {
			o.deleteSigner(idHash)
			return errors.Wrap(err, "failed to store entry in kvs for the passed signer")
		}
	}

	if verifier != nil {
		if err := o.RegisterVerifier(identity, verifier); err != nil {
			o.deleteSigner(idHash)
			return err
		}
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

	// check existence with a read lock
	idHash := identity.UniqueID()
	o.mutex.RLock()
	v, ok := o.verifiers[idHash]
	o.mutex.RUnlock()
	if ok {
		logger.Warnf("another verifier bound to [%s]:[%s][%s] from [%s]", idHash, GetIdentifier(v), GetIdentifier(verifier), string(v.DebugStack))
		return nil
	}

	// write lock
	o.mutex.Lock()

	// check again
	v, ok = o.verifiers[idHash]
	if ok {
		o.mutex.Unlock()
		logger.Warnf("another verifier bound to [%s]:[%s][%s] from [%s]", idHash, GetIdentifier(v), GetIdentifier(verifier), string(v.DebugStack))
		return nil
	}

	entry := VerifierEntry{Verifier: verifier}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		entry.DebugStack = debug.Stack()
	}
	o.verifiers[idHash] = entry
	o.mutex.Unlock()

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("register verifier to [%s]:[%s]", idHash, GetIdentifier(verifier))
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
	if err := o.kvs.Put(k, info); err != nil {
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

	if !o.kvs.Exists(k) {
		return nil, nil
	}
	var res []byte
	if err := o.kvs.Get(k, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (o *Service) IsMe(identity view.Identity) bool {
	idHash := identity.UniqueID()
	// check local cache
	o.mutex.RLock()
	_, ok := o.signers[idHash]
	o.mutex.RUnlock()
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
	o.mutex.RLock()
	entry, ok := o.signers[idHash]
	o.mutex.RUnlock()
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

		// write lock
		o.mutex.Lock()

		// check again
		entry, ok = o.signers[idHash]
		if ok {
			// found
			o.mutex.Unlock()
		} else {
			// not found
			o.signers[idHash] = entry
			o.mutex.Unlock()
		}
	} else {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("signer for [%s] found", idHash)
		}
	}
	return entry.Signer, nil
}

func (o *Service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	idHash := identity.UniqueID()

	o.mutex.RLock()
	entry, ok := o.verifiers[idHash]
	o.mutex.RUnlock()
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

		newEntry := VerifierEntry{Verifier: verifier}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			entry.DebugStack = debug.Stack()
			logger.Debugf("add deserialized verifier for [%s]:[%s]", idHash, GetIdentifier(verifier))
		}
		// write lock
		o.mutex.Lock()

		// check again
		entry, ok = o.verifiers[idHash]
		if ok {
			// found
			o.mutex.Unlock()
		} else {
			// not found
			o.verifiers[idHash] = newEntry
			entry = newEntry
			o.mutex.Unlock()
		}
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

func (o *Service) registerSigner(id string, signer *SignerEntry) error {
	k, err := kvs.CreateCompositeKey("sigService", []string{"signer", id})
	if err != nil {
		return errors.Wrap(err, "failed to create composite key to store entry in kvs")
	}
	err = o.kvs.Put(k, signer)
	if err != nil {
		return errors.Wrap(err, "failed to store entry in kvs for the passed signer")
	}
	return nil
}

func (o *Service) deleteSigner(id string) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	delete(o.signers, id)
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

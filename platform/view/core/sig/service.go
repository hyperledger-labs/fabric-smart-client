/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sig

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("view-sdk.sig")

type service struct {
	sp           driver.ServiceProvider
	signers      map[string]driver.Signer
	verifiers    map[string]driver.Verifier
	deserializer Deserializer
	viewsSync    sync.RWMutex
}

func NewSignService(sp driver.ServiceProvider, deserializer Deserializer) *service {
	return &service{
		sp:           sp,
		signers:      map[string]driver.Signer{},
		verifiers:    map[string]driver.Verifier{},
		deserializer: deserializer,
	}
}

func (o *service) RegisterSigner(identity view.Identity, signer driver.Signer, verifier driver.Verifier) error {
	if signer == nil {
		return errors.New("invalid signer, expected a valid instance")
	}
	if verifier == nil {
		return errors.New("invalid verifier, expected a valid instance")
	}

	o.viewsSync.Lock()
	_, ok := o.signers[identity.UniqueID()]
	o.viewsSync.Unlock()
	if ok {
		logger.Warnf("another signer bound to %s [%s]", identity, debug.Stack())
		return nil
	}
	logger.Debugf("add signer for [id:%s]", identity.UniqueID())
	o.viewsSync.Lock()
	o.signers[identity.UniqueID()] = signer
	o.viewsSync.Unlock()

	return o.RegisterVerifier(identity, verifier)
}

func (o *service) RegisterVerifier(identity view.Identity, verifier driver.Verifier) error {
	if verifier == nil {
		return errors.New("invalid verifier, expected a valid instance")
	}

	o.viewsSync.Lock()
	v, ok := o.verifiers[identity.UniqueID()]
	o.viewsSync.Unlock()
	if ok {
		logger.Warnf("another verifier bound to [%s][%v]", identity.UniqueID(), v)
		return nil
	}
	logger.Debugf("add verifier for [%s]", identity.UniqueID())
	o.viewsSync.Lock()
	o.verifiers[identity.UniqueID()] = verifier
	o.viewsSync.Unlock()

	return nil
}

func (o *service) RegisterAuditInfo(identity view.Identity, info []byte) error {
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

func (o *service) GetAuditInfo(identity view.Identity) ([]byte, error) {
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

func (o *service) Info(id view.Identity) string {
	auditInfo, err := o.GetAuditInfo(id)
	if err != nil {
		logger.Debugf("failed getting audit info for [%s]", id)
		return fmt.Sprintf("unable to identify identity : [%s][%s]", id.UniqueID(), string(id))
	}
	info, err := o.deserializer.Info(id, auditInfo)
	if err != nil {
		logger.Debugf("failed getting info for [%s]", id)
		return fmt.Sprintf("unable to identify identity : [%s][%s]", id.UniqueID(), string(id))
	}
	return info
}

func (o *service) GetSigner(identity view.Identity) (driver.Signer, error) {
	o.viewsSync.Lock()
	signer, ok := o.signers[identity.UniqueID()]
	o.viewsSync.Unlock()
	if !ok {
		// ask the deserialiser
		if o.deserializer == nil {
			return nil, errors.Errorf("cannot find signer for [%s], no deserializer set", identity)
		}
		var err error
		signer, err = o.deserializer.DeserializeSigner(identity)
		if err != nil {
			return nil, errors.Wrapf(err, "failed deserializing identity for signer [%s]", identity)
		}
		o.viewsSync.Lock()
		o.signers[identity.UniqueID()] = signer
		o.viewsSync.Unlock()
	}
	return signer, nil
}

func (o *service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	o.viewsSync.Lock()
	verifier, ok := o.verifiers[identity.UniqueID()]
	o.viewsSync.Unlock()
	if !ok {
		// ask the deserialiser
		if o.deserializer == nil {
			return nil, errors.Errorf("cannot find verifier for [%s], no deserializer set", identity)
		}
		var err error
		verifier, err = o.deserializer.DeserializeVerifier(identity)
		if err != nil {
			return nil, errors.Wrapf(err, "failed deserializing identity for verifier %v", identity)
		}

		logger.Debugf("add verifier for [%s]", identity.UniqueID())
		o.viewsSync.Lock()
		o.verifiers[identity.UniqueID()] = verifier
		o.viewsSync.Unlock()
	}
	return verifier, nil
}

func (o *service) GetSigningIdentity(identity view.Identity) (driver.SigningIdentity, error) {
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

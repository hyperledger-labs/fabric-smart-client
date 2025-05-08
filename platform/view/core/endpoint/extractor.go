package endpoint

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

import (
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

func NewPKIExtractor() *PKIExtractor {
	return &PKIExtractor{
		publicKeyExtractors:    []driver.PublicKeyExtractor{},
		publicKeyIDSynthesizer: DefaultPublicKeyIDSynthesizer{},
	}
}

type PKIExtractor struct {
	pkiExtractorsLock      sync.RWMutex
	publicKeyExtractors    []driver.PublicKeyExtractor
	publicKeyIDSynthesizer driver.PublicKeyIDSynthesizer
}

func (r *PKIExtractor) AddPublicKeyExtractor(publicKeyExtractor driver.PublicKeyExtractor) error {
	r.pkiExtractorsLock.Lock()
	defer r.pkiExtractorsLock.Unlock()

	if publicKeyExtractor == nil {
		return errors.New("pki resolver should not be nil")
	}
	r.publicKeyExtractors = append(r.publicKeyExtractors, publicKeyExtractor)
	return nil
}

func (r *PKIExtractor) SetPublicKeyIDSynthesizer(publicKeyIDSynthesizer driver.PublicKeyIDSynthesizer) {
	r.publicKeyIDSynthesizer = publicKeyIDSynthesizer
}

func (r *PKIExtractor) PkiResolve(resolver *Resolver) []byte {
	resolver.PKILock.RLock()
	if len(resolver.PKI) != 0 {
		resolver.PKILock.RUnlock()
		return resolver.PKI
	}
	resolver.PKILock.RUnlock()

	resolver.PKILock.Lock()
	defer resolver.PKILock.Unlock()
	if len(resolver.PKI) == 0 {
		resolver.PKI = r.ExtractPKI(resolver.Id)
	}
	return resolver.PKI
}

func (r *PKIExtractor) ExtractPKI(id []byte) []byte {
	r.pkiExtractorsLock.RLock()
	defer r.pkiExtractorsLock.RUnlock()

	for _, extractor := range r.publicKeyExtractors {
		if pk, err := extractor.ExtractPublicKey(id); pk != nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("pki resolved for [%s]", id)
			}
			return r.publicKeyIDSynthesizer.PublicKeyID(pk)
		} else {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("pki not resolved by [%s] for [%s]: [%s]", getIdentifier(extractor), id, err)
			}
		}
	}
	logger.Warnf("cannot resolve pki for [%s]", id)
	return nil
}

func getIdentifier(f any) string {
	if f == nil {
		return "<nil view>"
	}
	t := reflect.TypeOf(f)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() + "/" + t.Name()
}

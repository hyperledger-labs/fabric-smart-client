/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sig

import (
	"sync"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	sig2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

type Deserializer interface {
	DeserializeVerifier(raw []byte) (driver.Verifier, error)
	DeserializeSigner(raw []byte) (driver.Signer, error)
	Info(raw []byte, auditInfo []byte) (string, error)
}

type deserializer struct {
	deserializersMutex sync.RWMutex
	deserializers      []Deserializer
}

func NewMultiplexDeserializer() (*deserializer, error) {
	return &deserializer{
		deserializers: []Deserializer{},
	}, nil
}

func (d *deserializer) AddDeserializer(newD Deserializer) {
	d.deserializersMutex.Lock()
	d.deserializers = append(d.deserializers, newD)
	d.deserializersMutex.Unlock()
}

func (d *deserializer) DeserializeVerifier(raw []byte) (driver.Verifier, error) {
	var errs []error

	copyDeserial := d.threadSafeCopyDeserializers()
	for _, des := range copyDeserial {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("trying deserialization with [%v]", des)
		}
		v, err := des.DeserializeVerifier(raw)
		if err == nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("trying deserialization with [%v] succeeded", des)
			}
			return v, nil
		}

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("trying deserialization with [%v] failed", des)
		}
		errs = append(errs, err)
	}

	return nil, errors.Errorf("failed deserialization [%v]", errs)
}

func (d *deserializer) DeserializeSigner(raw []byte) (driver.Signer, error) {
	var errs []error

	copyDeserial := d.threadSafeCopyDeserializers()
	for _, des := range copyDeserial {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("trying signer deserialization with [%s]", des)
		}
		v, err := des.DeserializeSigner(raw)
		if err == nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("trying signer deserialization with [%s] succeeded", des)
			}
			return v, nil
		}

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("trying signer deserialization with [%s] failed [%s]", des, err)
		}
		errs = append(errs, err)
	}

	return nil, errors.Errorf("failed signer deserialization [%v]", errs)
}

func (d *deserializer) Info(raw []byte, auditInfo []byte) (string, error) {
	var errs []error

	copyDeserial := d.threadSafeCopyDeserializers()
	for _, des := range copyDeserial {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("trying info deserialization with [%v]", des)
		}
		v, err := des.Info(raw, auditInfo)
		if err == nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("trying info deserialization with [%v] succeeded", des)
			}
			return v, nil
		}

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("trying info deserialization with [%v] failed", des)
		}
		errs = append(errs, err)
	}

	return "", errors.Errorf("failed info deserialization [%v]", errs)
}

func (d *deserializer) threadSafeCopyDeserializers() []Deserializer {
	d.deserializersMutex.RLock()
	res := make([]Deserializer, len(d.deserializers))
	copy(res, d.deserializers)
	d.deserializersMutex.RUnlock()
	return res
}

func NewDeserializer() (Deserializer, error) {
	des, err := NewMultiplexDeserializer()
	if err != nil {
		return nil, errors.Wrap(err, "failed loading sig verifier deserializer service")
	}
	des.AddDeserializer(&x509.Deserializer{})
	return des, nil
}

func NewDeserializerManager(kvss *kvs.KVS) (*sig2.Service, driver2.DeserializerManager) {
	des := sig2.NewMultiplexDeserializer()
	sigService := sig2.NewService(des, kvss)
	return sigService, des
}

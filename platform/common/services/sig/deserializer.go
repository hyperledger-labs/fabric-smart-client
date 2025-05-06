/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sig

import (
	"sync"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/pkg/errors"
)

type Deserializer = driver2.SigDeserializer

type MultiplexDeserializer struct {
	deserializersMutex sync.RWMutex
	deserializers      []Deserializer
}

func NewMultiplexDeserializer() *MultiplexDeserializer {
	return &MultiplexDeserializer{
		deserializers: []Deserializer{},
	}
}

func (d *MultiplexDeserializer) AddDeserializer(newD Deserializer) {
	d.deserializersMutex.Lock()
	d.deserializers = append(d.deserializers, newD)
	d.deserializersMutex.Unlock()
}

func (d *MultiplexDeserializer) DeserializeVerifier(raw []byte) (driver.Verifier, error) {
	var errs []error

	copyDeserial := d.threadSafeCopyDeserializers()
	for _, des := range copyDeserial {
		logger.Debugf("trying deserialization with [%v]", des)
		v, err := des.DeserializeVerifier(raw)
		if err == nil {
			logger.Debugf("trying deserialization with [%v] succeeded", des)
			return v, nil
		}

		logger.Debugf("trying deserialization with [%v] failed", des)
		errs = append(errs, err)
	}

	return nil, errors.Errorf("failed deserialization [%v]", errs)
}

func (d *MultiplexDeserializer) DeserializeSigner(raw []byte) (driver.Signer, error) {
	var errs []error

	copyDeserial := d.threadSafeCopyDeserializers()
	for _, des := range copyDeserial {
		logger.Debugf("trying signer deserialization with [%s]", des)
		v, err := des.DeserializeSigner(raw)
		if err == nil {
			logger.Debugf("trying signer deserialization with [%s] succeeded", des)
			return v, nil
		}

		logger.Debugf("trying signer deserialization with [%s] failed [%s]", des, err)
		errs = append(errs, err)
	}

	return nil, errors.Errorf("failed signer deserialization [%v]", errs)
}

func (d *MultiplexDeserializer) Info(raw []byte, auditInfo []byte) (string, error) {
	var errs []error

	copyDeserial := d.threadSafeCopyDeserializers()
	for _, des := range copyDeserial {
		logger.Debugf("trying info deserialization with [%v]", des)
		v, err := des.Info(raw, auditInfo)
		if err == nil {
			logger.Debugf("trying info deserialization with [%v] succeeded", des)
			return v, nil
		}

		logger.Debugf("trying info deserialization with [%v] failed", des)
		errs = append(errs, err)
	}

	return "", errors.Errorf("failed info deserialization [%v]", errs)
}

func (d *MultiplexDeserializer) threadSafeCopyDeserializers() []Deserializer {
	d.deserializersMutex.RLock()
	res := make([]Deserializer, len(d.deserializers))
	copy(res, d.deserializers)
	d.deserializersMutex.RUnlock()
	return res
}

func NewDeserializer() (Deserializer, error) {
	des := NewMultiplexDeserializer()
	des.AddDeserializer(&x509.Deserializer{})
	return des, nil
}

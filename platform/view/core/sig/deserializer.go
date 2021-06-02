/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package sig

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
)

type Deserializer interface {
	DeserializeVerifier(raw []byte) (api.Verifier, error)
	DeserializeSigner(raw []byte) (api.Signer, error)
	Info(raw []byte, auditInfo []byte) (string, error)
}

type deserializer struct {
	sp            api.ServiceProvider
	deserializers []Deserializer
}

func NewMultiplexDeserializer(sp api.ServiceProvider) (*deserializer, error) {
	return &deserializer{
		sp:            sp,
		deserializers: []Deserializer{},
	}, nil
}

func (d *deserializer) AddDeserializer(newD Deserializer) {
	d.deserializers = append(d.deserializers, newD)
}

func (d *deserializer) DeserializeVerifier(raw []byte) (api.Verifier, error) {
	var errs []error
	for _, des := range d.deserializers {
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

func (d *deserializer) DeserializeSigner(raw []byte) (api.Signer, error) {
	var errs []error
	for _, des := range d.deserializers {
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

func (d *deserializer) Info(raw []byte, auditInfo []byte) (string, error) {
	var errs []error
	for _, des := range d.deserializers {
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

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var (
	ServiceNotFound = errors.New("service not found")
)

type ServiceProvider struct {
	services   []interface{}
	serviceMap map[reflect.Type]interface{}
	lock       sync.Mutex
}

func New() *ServiceProvider {
	return &ServiceProvider{
		services:   []interface{}{},
		serviceMap: map[reflect.Type]interface{}{},
	}
}

func (sp *ServiceProvider) GetService(v interface{}) (interface{}, error) {
	sp.lock.Lock()
	defer sp.lock.Unlock()

	var typ reflect.Type
	switch t := v.(type) {
	case reflect.Type:
		typ = t
	default:
		typ = reflect.TypeOf(v)
	}

	switch typ.Kind() {
	case reflect.Struct:
		// nothing to do here
	default:
		typ = typ.Elem()
	}

	service, ok := sp.serviceMap[typ]
	if !ok {
		switch typ.Kind() {
		case reflect.Interface:
			for _, s := range sp.services {
				if reflect.TypeOf(s).Implements(typ) {
					sp.serviceMap[typ] = s
					return s, nil
				}
			}
		default:
			for _, s := range sp.services {
				if typ.AssignableTo(reflect.TypeOf(s).Elem()) {
					sp.serviceMap[typ] = s
					return s, nil
				}
			}
		}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			return nil, errors.Errorf("service [%s/%s] not found in [%v]", typ.PkgPath(), typ.Name(), sp.String())
		}
		return nil, ServiceNotFound

	}
	return service, nil
}

func (sp *ServiceProvider) RegisterService(service interface{}) error {
	sp.lock.Lock()
	defer sp.lock.Unlock()

	logger.Debugf("Register Service [%s]", logging.Identifier(service))
	sp.services = append(sp.services, service)

	return nil
}

func (sp *ServiceProvider) String() string {
	res := "services ["
	for _, service := range sp.services {
		res += logging.Identifier(service).String() + ", "
	}
	return res + "]"
}

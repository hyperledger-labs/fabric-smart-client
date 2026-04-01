/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// ServiceProvider is responsible for managing and providing services.
type ServiceProvider struct {
	services   []any
	serviceMap map[reflect.Type]any
	lock       sync.Mutex
}

// NewServiceProvider returns a new instance of the service provider.
func NewServiceProvider() *ServiceProvider {
	return &ServiceProvider{
		services:   []any{},
		serviceMap: map[reflect.Type]any{},
	}
}

// GetService returns the service of the given type.
func (sp *ServiceProvider) GetService(v any) (any, error) {
	sp.lock.Lock()
	defer sp.lock.Unlock()

	var typ reflect.Type
	switch t := v.(type) {
	case reflect.Type:
		typ = t
	default:
		typ = reflect.TypeOf(v)
	}

	if service, ok := sp.serviceMap[typ]; ok {
		if service == nil {
			return nil, errors.Wrapf(ErrServiceNotFound, "service [%s] not found", typ.String())
		}
		return service, nil
	}

	var found any
	// search
	for _, s := range sp.services {
		styp := reflect.TypeOf(s)

		// Match 1: direct
		if styp.AssignableTo(typ) {
			found = s
			break
		}

		// Match 2: if requested is pointer to interface
		if typ.Kind() == reflect.Ptr && typ.Elem().Kind() == reflect.Interface && styp.AssignableTo(typ.Elem()) {
			found = s
			break
		}

		// Match 3: if requested is interface
		if typ.Kind() == reflect.Interface && styp.AssignableTo(typ) {
			found = s
			break
		}

		// Match 4: struct match (typ is struct, s is ptr to struct)
		if typ.Kind() == reflect.Struct && styp.Kind() == reflect.Ptr && styp.Elem() == typ {
			found = s
			break
		}

		// Match 5: pointer match (typ is ptr to struct, s is struct)
		if typ.Kind() == reflect.Ptr && typ.Elem().Kind() == reflect.Struct && styp == typ.Elem() {
			found = s
			break
		}
	}

	if found != nil {
		sp.serviceMap[typ] = found
		return found, nil
	}

	// Cache the miss
	sp.serviceMap[typ] = nil

	return nil, errors.Wrapf(ErrServiceNotFound, "service [%s] not found", typ.String())
}

// RegisterService registers a service in the service provider.
func (sp *ServiceProvider) RegisterService(service any) error {
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

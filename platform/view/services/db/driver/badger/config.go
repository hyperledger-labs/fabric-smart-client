/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

func copy(oldValue interface{}, newValue interface{}, config driver.Config) {
	if config == nil {
		return
	}
	entityType := reflect.TypeOf(oldValue).Elem()
	for i := 0; i < entityType.NumField(); i++ {
		value := entityType.Field(i)
		oldField := reflect.ValueOf(oldValue).Elem().Field(i)
		newField := reflect.ValueOf(newValue).FieldByName(value.Name)
		if config.IsSet(value.Name) {
			if value.Type.Kind() == reflect.Struct {
				copy(oldField.Addr().Interface(), newField.Interface(), config)
			} else {
				logger.Debugf("set badger opts [%s] to [%v]", value.Name, newField)
				oldField.Set(newField)
			}
		}
	}
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vfsdk

import (
	errors2 "errors"
	"reflect"

	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/pkg/errors"
	"go.uber.org/dig"
)

var logger = logging.MustGetLogger("base-container")

var viewFactoryInterface = reflect.TypeOf(new(view.Factory)).Elem()
var errorInterface = reflect.TypeOf(new(error)).Elem()
var factoryEntryType = reflect.TypeOf(&factoryEntry{})

var nilError = reflect.Zero(reflect.TypeOf((*error)(nil)).Elem())

type vfContainer struct{ *dig.Container }

func NewFromContainer(c *dig.Container) *vfContainer {
	return &vfContainer{Container: c}
}

func NewContainer(options ...dig.Option) *vfContainer {
	return NewFromContainer(dig.New(options...))
}

func (c *vfContainer) Provide(constructor any, opts ...common.ProvideOption) error {
	digOpts := make([]dig.ProvideOption, 0, len(opts))
	vfOpts := make([]FactoryOption, 0, len(opts))
	for _, opt := range opts {
		if o, ok := opt.(dig.ProvideOption); ok {
			digOpts = append(digOpts, o)
		} else if o, ok := opt.(FactoryOption); ok {
			vfOpts = append(vfOpts, o)
		}
	}

	if len(vfOpts) == 0 {
		return c.Container.Provide(constructor, digOpts...)
	}

	constructorTyp := reflect.TypeOf(constructor)

	if err := validateConstructorType(constructorTyp); err != nil {
		return err
	}
	ins := make([]reflect.Type, constructorTyp.NumIn())
	for i := 0; i < constructorTyp.NumIn(); i++ {
		ins[i] = constructorTyp.In(i)
	}
	outs := []reflect.Type{
		factoryEntryType,
		errorInterface,
	}
	fnType := reflect.FuncOf(ins, outs, false)

	fn := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		results := reflect.ValueOf(constructor).Call(args)
		if !results[0].IsNil() {
			results[0] = reflect.ValueOf(newEntry(results[0].Interface(), vfOpts))
		}
		if len(results) == 1 {
			results = append(results, nilError)
		}
		return results
	})
	return errors2.Join(
		c.Container.Provide(constructor, digOpts...),
		c.Container.Provide(fn.Interface(), append(digOpts, dig.Group("view-factories"))...),
	)
}

func (c *vfContainer) Visualize() string {
	return digutils.Visualize(c.Container)
}

type factoryEntry struct {
	fids       []string
	initiators []any
	factory    driver.Factory
}

func newEntry(factory any, opts []FactoryOption) *factoryEntry {
	entry := &factoryEntry{factory: factory.(driver.Factory)}
	for _, opt := range opts {
		opt(entry)
	}
	return entry
}

func validateConstructorType(typ reflect.Type) error {
	if typ.Kind() != reflect.Func {
		return errors.Errorf("passed type is not a constructor [%v]", typ)
	}
	if typ.NumOut() > 2 || typ.NumOut() == 0 {
		return errors.Errorf("wrong number of arguments returned: %d", typ.NumOut())
	}
	if !typ.Out(0).Implements(viewFactoryInterface) {
		return errors.Errorf("first param should implement view.Factory")
	}
	if typ.NumOut() == 2 && !typ.Out(1).Implements(errorInterface) {
		return errors.Errorf("constructor must either return one factory or one factory and an error")
	}
	return nil
}

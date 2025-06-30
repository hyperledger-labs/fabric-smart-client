/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"bytes"
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"go.uber.org/dig"
)

type invoker interface {
	Invoke(function interface{}, opts ...dig.InvokeOption) error
}

func Visualize(c *dig.Container) string {
	var w bytes.Buffer
	if err := dig.Visualize(c, &w); err != nil {
		return fmt.Sprintf("could not visualize: [%v]", err)
	}
	return (&w).String()
}

func Register[T any](c invoker) error {
	// Temporary workaround for services that are imported still using the registry
	err := c.Invoke(func(registry services.Registry, service T) error {
		return registry.RegisterService(service)
	})
	if err != nil {
		debug.PrintStack()
		return fmt.Errorf("failed registering type %T: %+v", *new(T), err)
	}
	return nil
}

func ProvideAll(c *dig.Container, constructors ...interface{}) error {
	errs := make([]error, len(constructors))
	for i, constructor := range constructors {
		errs[i] = c.Provide(constructor)
	}
	return errors.Join(errs...)
}

func Identity[T any]() func(T) T {
	return func(t T) T {
		return t
	}
}

type HandlerProvider[K comparable, C any] struct {
	Type K
	New  C
}

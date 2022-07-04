/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"

	"github.com/pkg/errors"
)

type StartableWithContext interface {
	Start(ctx context.Context) error
}

type StartableWithoutContext interface {
	Start() error
}

type Stoppable interface {
	Stop() error
}

type Service interface {
	StartableWithContext
	Stoppable
}

type ServiceWithoutContext interface {
	StartableWithoutContext
	Stoppable
}

type autoStopper struct {
	instance Service
}

func NewServiceWithAutoStop(instance Service) (Service, error) {
	if instance == nil {
		return nil, errors.Errorf("instance must be set")
	}
	return &autoStopper{instance: instance}, nil
}

func (a *autoStopper) Start(ctx context.Context) error {

	errs := make(chan error, 1)

	go func() {
		errs <- a.instance.Start(ctx)
	}()

	go func() {
		select {
		case <-ctx.Done():
			errs <- a.Stop()
		}
	}()

	return nil
}

func (a *autoStopper) Stop() error {
	return a.instance.Stop()
}

type stoppable struct {
	instance StartableWithContext
}

func NewStoppable(instance StartableWithContext) (Service, error) {
	if instance == nil {
		return nil, errors.Errorf("instance must be set")
	}
	return &stoppable{instance: instance}, nil
}

func (a *stoppable) Start(ctx context.Context) error {
	return a.instance.Start(ctx)
}

func (a *stoppable) Stop() error {
	// closed via context
	// no-op
	return nil
}

type stopByContext struct {
	instance ServiceWithoutContext
}

func NewServiceWithoutContext(instance ServiceWithoutContext) (Service, error) {
	if instance == nil {
		return nil, errors.Errorf("instance must be set")
	}
	return &stopByContext{instance: instance}, nil
}

func (a *stopByContext) Start(_ context.Context) error {
	return a.instance.Start()
}

func (a *stopByContext) Stop() error {
	return a.instance.Stop()
}

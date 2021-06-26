/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracker

import (
	"reflect"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("view-sdk.tracker")

const (
	RUNNING = iota
	DONE
	ERROR
)

type ViewStatus struct {
	Status     int
	LastReport string
}

type ViewTracker interface {
	Status() int

	Report(msg string)

	LatestReport() string

	Error(err error)

	Done(result interface{})

	ViewStatus() *ViewStatus
}

func GetViewTracker(ctx view2.ServiceProvider) (ViewTracker, error) {
	s, err := ctx.GetService(reflect.TypeOf((*ViewTracker)(nil)))
	if err != nil {
		return nil, err
	}
	return s.(ViewTracker), nil
}

type defaultTracker struct {
	lastReport string
	err        error
	result     interface{}
	status     int
}

func NewTracker() *defaultTracker {
	return &defaultTracker{status: RUNNING}
}

func (d *defaultTracker) ViewStatus() *ViewStatus {
	return &ViewStatus{
		Status:     d.status,
		LastReport: d.lastReport,
	}
}

func (d *defaultTracker) Status() int {
	return d.status
}

func (d *defaultTracker) Report(msg string) {
	logger.Debugf(msg)
	d.lastReport = msg
}

func (d *defaultTracker) LatestReport() string {
	return d.lastReport
}

func (d *defaultTracker) Error(err error) {
	logger.Errorf(err.Error())
	d.status = ERROR
	d.lastReport = err.Error()
	d.err = err
}

func (d *defaultTracker) Done(result interface{}) {
	d.status = DONE
	d.result = result
}

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package disabled

import (
	"time"
)

type Disabled struct {
}

func New() *Disabled {
	return &Disabled{}
}

func (d *Disabled) Start(spanName string) {

}

func (d *Disabled) StartAt(spanName string, when time.Time) {

}

func (d *Disabled) AddEvent(spanName string, eventName string) {

}

func (d *Disabled) AddEventAt(spanName string, eventName string, when time.Time) {

}

func (d *Disabled) AddError(spanName string, err error) {

}

func (d *Disabled) End(spanName string, attrs ...string) {

}

func (d *Disabled) EndAt(spanName string, when time.Time, attrs ...string) {

}

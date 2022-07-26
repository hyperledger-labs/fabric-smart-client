/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package assert

import (
	"fmt"

	"github.com/test-go/testify/assert"
)

type panickier struct {
	releasers []func()
}

func (p *panickier) Errorf(format string, args ...interface{}) {
	if len(p.releasers) != 0 {
		for _, releaser := range p.releasers {
			releaser()
		}
	}
	panic(fmt.Sprintf(format, args...))
}

func NotNil(object interface{}, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.NotNil(&panickier{releasers: releasers}, object, ma...)
}

// NoError checks that the passed error is nil, it panics otherwise
func NoError(err error, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.NoError(&panickier{releasers: releasers}, err, ma...)
}

// Error checks that the passed error is not nil, it panics otherwise
func Error(err error, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.Error(&panickier{releasers: releasers}, err, ma...)
}

func NotEmpty(o interface{}, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.NotEmpty(&panickier{releasers: releasers}, o, ma...)
}

// Equal checks that actual is as expected, it panics otherwise
func Equal(expected, actual interface{}, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.Equal(&panickier{releasers: releasers}, expected, actual, ma...)
}

func NotEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.NotEqual(&panickier{releasers: releasers}, expected, actual, ma...)
}

func True(value bool, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.True(&panickier{releasers: releasers}, value, ma...)
}

func False(value bool, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.False(&panickier{releasers: releasers}, value, ma...)
}

func Fail(failureMessage string, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.Fail(&panickier{releasers: releasers}, failureMessage, ma...)
}

func extractReleasers(msgAndArgs ...interface{}) ([]interface{}, []func()) {
	var output []interface{}
	var releasers []func()
	for _, arg := range msgAndArgs {
		switch arg := arg.(type) {
		case func():
			releasers = append(releasers, arg)
		default:
			output = append(output, arg)
		}
	}
	return output, releasers
}

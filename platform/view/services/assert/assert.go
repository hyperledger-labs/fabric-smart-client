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

func NoError(err error, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.NoError(&panickier{releasers: releasers}, err, ma...)
}

func NotEmpty(o interface{}, msgAndArgs ...interface{}) {
	ma, releasers := extractReleasers(msgAndArgs...)
	assert.NotEmpty(&panickier{releasers: releasers}, o, ma...)
}

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
		switch arg.(type) {
		case func():
			releasers = append(releasers, arg.(func()))
		default:
			output = append(output, arg)
		}
	}
	return output, releasers
}
